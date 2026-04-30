package vfs

import (
	"sync"
	"testing"
	"time"

	"github.com/yinzhenyu/skills/qrypt/internal/driver"
)

// TestMergeRemoteChanges_SkipRecentlySynced 验证上传后 API 索引延迟场景：
// 文件刚上传成功，但 API 还未索引到，MergeRemoteChanges 收到空列表时不应删除本地节点。
func TestMergeRemoteChanges_SkipRecentlySynced(t *testing.T) {
	fs := &QryptFS{
		nodes:     sync.Map{},
		fidNodes:  sync.Map{},
	}

	// 创建根目录
	root := &node{
		fid:         "root_fid",
		currentPath: "/",
		isFolder:    true,
		children:    make(map[string]*node),
	}
	fs.storeNode("/", root)

	// 创建刚上传完成的文件（lastUploadTime = 2秒前）
	recentFile := &node{
		fid:             "file_fid",
		parentFid:       "root_fid",
		name:            "myfile.txt",
		currentPath:     "/myfile.txt",
		size:            1024,
		isDirty:         false,
		lastUploadTime:  time.Now().Add(-2 * time.Second), // 2秒前上传完成
	}
	fs.storeNode("/myfile.txt", recentFile)

	// 远程列表为空（模拟 API 还没索引到新文件）
	remoteFiles := []driver.File{}

	// 调用 MergeRemoteChanges
	fs.MergeRemoteChanges("/", "root_fid", remoteFiles)

	// 验证文件仍然存在（不应被误删）
	if _, ok := fs.nodes.Load("/myfile.txt"); !ok {
		t.Error("recently synced file was incorrectly deleted by MergeRemoteChanges")
	}
	if _, ok := root.children["myfile.txt"]; !ok {
		t.Error("recently synced file was removed from parent.children")
	}
}

// TestMergeRemoteChanges_DeleteStaleRemoteFile 验证正常删除逻辑：
// 文件同步很久后，远程确实删除了，MergeRemoteChanges 应该删除本地节点。
func TestMergeRemoteChanges_DeleteStaleRemoteFile(t *testing.T) {
	fs := &QryptFS{
		nodes:     sync.Map{},
		fidNodes:  sync.Map{},
	}

	root := &node{
		fid:         "root_fid",
		currentPath: "/",
		isFolder:    true,
		children:    make(map[string]*node),
	}
	fs.storeNode("/", root)

	// 创建很久之前同步的文件（lastUploadTime = 6分钟前）
	oldFile := &node{
		fid:             "file_fid",
		parentFid:       "root_fid",
		name:            "old_file.txt",
		currentPath:     "/old_file.txt",
		size:            512,
		isDirty:         false,
		lastUploadTime:  time.Now().Add(-6 * time.Minute), // 6分钟前同步
	}
	fs.storeNode("/old_file.txt", oldFile)

	// 远程列表为空（文件确实被删除了）
	remoteFiles := []driver.File{}

	fs.MergeRemoteChanges("/", "root_fid", remoteFiles)

	// 验证文件已被删除
	if _, ok := fs.nodes.Load("/old_file.txt"); ok {
		t.Error("stale file should have been deleted but still exists")
	}
	if _, ok := root.children["old_file.txt"]; ok {
		t.Error("stale file should have been removed from parent.children")
	}
}

// TestMergeRemoteChanges_SkipSyncInProgress 验证正在同步的文件不被删除
func TestMergeRemoteChanges_SkipSyncInProgress(t *testing.T) {
	fs := &QryptFS{
		nodes:     sync.Map{},
		fidNodes:  sync.Map{},
	}

	root := &node{
		fid:         "root_fid",
		currentPath: "/",
		isFolder:    true,
		children:    make(map[string]*node),
	}
	fs.storeNode("/", root)

	// 正在同步的文件（syncQueued = true）
	syncingFile := &node{
		fid:             "file_fid",
		parentFid:       "root_fid",
		name:            "syncing.txt",
		currentPath:     "/syncing.txt",
		size:            256,
		isDirty:         false,
		syncQueued:      true, // 正在同步
		lastUploadTime:  time.Time{}, // 零值
	}
	fs.storeNode("/syncing.txt", syncingFile)

	remoteFiles := []driver.File{}

	fs.MergeRemoteChanges("/", "root_fid", remoteFiles)

	if _, ok := fs.nodes.Load("/syncing.txt"); !ok {
		t.Error("file with syncQueued=true was incorrectly deleted")
	}
}

// TestMergeRemoteChanges_SkipLocalFidFile 验证 local_ fid 前缀保护：
// 通过 ensureParentDirExists 等路径创建的节点 source 未设置（空字符串），
// 但 fid 以 local_ 开头表示从未上传过，MergeRemoteChanges 不应删除。
func TestMergeRemoteChanges_SkipLocalFidFile(t *testing.T) {
	fs := &QryptFS{
		nodes:    sync.Map{},
		fidNodes: sync.Map{},
	}

	root := &node{
		fid:         "root_fid",
		currentPath: "/",
		isFolder:    true,
		children:    make(map[string]*node),
	}
	fs.storeNode("/", root)

	// 文件由 ensureParentDirExists 创建：source 为空，fid 以 local_ 开头
	// isDirty=false, syncQueued=false, lastUploadTime=零值（从未上传过）
	localFidFile := &node{
		fid:         "local_newfile.txt_1234567890", // local_ 前缀
		parentFid:   "root_fid",
		name:        "newfile.txt",
		currentPath: "/newfile.txt",
		size:        0,
		isDirty:     false,
		source:      "",          // 未设置！
		syncQueued:  false,
		// lastUploadTime 零值 — 从未上传过
	}
	fs.storeNode("/newfile.txt", localFidFile)

	// 远程列表为空（文件从未上传，自然不在远程）
	remoteFiles := []driver.File{}

	fs.MergeRemoteChanges("/", "root_fid", remoteFiles)

	// 验证文件不被误删
	if _, ok := fs.nodes.Load("/newfile.txt"); !ok {
		t.Error("local_ fid file with empty source was incorrectly deleted by MergeRemoteChanges")
	}
	if _, ok := root.children["newfile.txt"]; !ok {
		t.Error("local_ fid file was removed from parent.children")
	}
}

// TestMergeRemoteChanges_UpdateRemoteFile 验证远程文件更新能正常同步
func TestMergeRemoteChanges_UpdateRemoteFile(t *testing.T) {
	fs := &QryptFS{
		nodes:     sync.Map{},
		fidNodes:  sync.Map{},
	}

	root := &node{
		fid:         "root_fid",
		currentPath: "/",
		isFolder:    true,
		children:    make(map[string]*node),
	}
	fs.storeNode("/", root)

	// 本地文件，上次同步时间较早
	localFile := &node{
		fid:               "file_fid",
		parentFid:         "root_fid",
		name:              "update_test.txt",
		currentPath:       "/update_test.txt",
		size:              100,
		baseServerMtime:   1000000, // 很早的时间
		isDirty:           false,
		lastUploadTime:    time.Now().Add(-60 * time.Second), // 1分钟前上传
	}
	fs.storeNode("/update_test.txt", localFile)

	// 远程文件有更新（mtime 更大）
	remoteFiles := []driver.File{
		{
			Fid:      "file_fid",
			FileName: "update_test.txt", // 注意：实际场景中是加密名称
			UpdatedAt: 2000000, // 比本地 baseServerMtime 新
			File:     true,
		},
	}

	// 注意：这个测试需要 mock cipher 才能完整运行
	// 这里主要验证函数不会 panic
	fs.MergeRemoteChanges("/", "root_fid", remoteFiles)

	// 文件应该仍然存在
	if _, ok := fs.nodes.Load("/update_test.txt"); !ok {
		t.Error("file should still exist after merge")
	}
}

// TestMergeRemoteChanges_UploadedButNotIndexed 验证上传完成后 API 未索引场景：
// 文件上传成功后 source 保持 "local"（syncFile 不再设置 source="remote"），
// 当远程列表中没有该文件时，MergeRemoteChanges 不应删除本地节点；
// 当远程列表中出现该文件（FID 匹配）时，source 应自动转为 "remote"。
func TestMergeRemoteChanges_UploadedButNotIndexed(t *testing.T) {
	fs := &QryptFS{
		nodes:    sync.Map{},
		fidNodes: sync.Map{},
	}

	root := &node{
		fid:         "root_fid",
		currentPath: "/",
		isFolder:    true,
		children:    make(map[string]*node),
	}
	fs.storeNode("/", root)

	// 模拟上传完成的文件：syncFile 已执行，fid 变为真实 FID，但 source 仍为 "local"
	// （syncFile 不再设置 source="remote"，由 MergeRemoteChanges 确认后转换）
	uploadedFile := &node{
		fid:             "server_fid_123",
		parentFid:       "root_fid",
		name:            "app.js",
		currentPath:     "/app.js",
		size:            4096,
		isDirty:         false,
		syncQueued:      false,
		source:          "local", // 上传后仍为 "local"（修复后的预期行为）
		expectedFid:     "server_fid_123",
		lastUploadTime:  time.Now().Add(-5 * time.Second), // 5秒前上传完成
		baseServerMtime: 0,                                // 尚未被远程确认
	}
	fs.storeNode("/app.js", uploadedFile)

	// 场景1：远程列表为空（API 未索引到文件）
	remoteFiles := []driver.File{}
	fs.MergeRemoteChanges("/", "root_fid", remoteFiles)

	// 文件不应被误删
	if _, ok := fs.nodes.Load("/app.js"); !ok {
		t.Fatal("uploaded file was incorrectly deleted when remote list is empty (API not indexed yet)")
	}
	// source 应仍为 "local"
	if v, ok := fs.nodes.Load("/app.js"); ok {
		n := v.(*node)
		if n.source != "local" {
			t.Errorf("source should remain 'local' when not in remote list, got '%s'", n.source)
		}
	}

	// 场景2：远程列表中出现该文件（API 已索引），FID 匹配
	remoteFiles = []driver.File{
		{
			Fid:      "server_fid_123",
			FileName: "app.js",
			UpdatedAt: time.Now().UnixMilli(),
			File:     true,
		},
	}
	fs.MergeRemoteChanges("/", "root_fid", remoteFiles)

	// 文件应存在
	if _, ok := fs.nodes.Load("/app.js"); !ok {
		t.Fatal("file should still exist after remote confirms it")
	}
	// source 应自动转为 "remote"
	if v, ok := fs.nodes.Load("/app.js"); ok {
		n := v.(*node)
		if n.source != "remote" {
			t.Errorf("source should be 'remote' after confirmed in remote list, got '%s'", n.source)
		}
	}
}
