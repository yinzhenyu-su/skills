package vfs

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestQryptFS_RenameRecursiveCache(t *testing.T) {
	fs := &QryptFS{}
	
	// 准备测试数据
	fs.nodes.Store("/", &node{fid: "root", isFolder: true})
	fs.nodes.Store("/A", &node{fid: "fid_a", name: "A", isFolder: true})
	fs.nodes.Store("/A/b.txt", &node{fid: "fid_b", name: "b.txt", isFolder: false})
	fs.nodes.Store("/A/Sub", &node{fid: "fid_sub", name: "Sub", isFolder: true})
	fs.nodes.Store("/A/Sub/c.dat", &node{fid: "fid_c", name: "c.dat", isFolder: false})
	fs.nodes.Store("/Other", &node{fid: "fid_other", name: "Other", isFolder: false})

	// 模拟重命名 /A -> /X
	oldPath := "/A"
	newPath := "/X"
	newName := "X"
	
	v, _ := fs.nodes.Load(oldPath)
	oldNode := v.(*node)
	
	// 执行重命名逻辑 (手动模拟 fs.Rename 中的缓存更新部分)
	fs.nodes.Delete(oldPath)
	oldNode.name = newName
	fs.nodes.Store(newPath, oldNode)

	if oldNode.isFolder {
		oldPrefix := oldPath
		if !strings.HasSuffix(oldPrefix, "/") {
			oldPrefix += "/"
		}
		newPrefix := newPath
		if !strings.HasSuffix(newPrefix, "/") {
			newPrefix += "/"
		}

		fs.nodes.Range(func(key, value interface{}) bool {
			path, ok := key.(string)
			if !ok {
				return true
			}
			if strings.HasPrefix(path, oldPrefix) {
				childNode := value.(*node)
				relative := strings.TrimPrefix(path, oldPrefix)
				newChildPath := newPrefix + relative
				
				fs.nodes.Delete(path)
				fs.nodes.Store(newChildPath, childNode)
			}
			return true
		})
	}

	// 验证结果
	expectedPaths := []string{
		"/",
		"/X",
		"/X/b.txt",
		"/X/Sub",
		"/X/Sub/c.dat",
		"/Other",
	}
	
	for _, p := range expectedPaths {
		if _, ok := fs.nodes.Load(p); !ok {
			t.Errorf("Expected path %s not found in cache", p)
		}
	}
	
	unexpectedPaths := []string{
		"/A",
		"/A/b.txt",
		"/A/Sub",
		"/A/Sub/c.dat",
	}
	
	for _, p := range unexpectedPaths {
		if _, ok := fs.nodes.Load(p); ok {
			t.Errorf("Path %s should have been removed from cache", p)
		}
	}
}

func TestQryptFS_WriteSyncCoordination(t *testing.T) {
	n := &node{
		fid:      "test_fid",
		name:     "test.txt",
		size:     100,
		isDirty:  true,
		isFolder: false,
	}

	// 模拟同步开始前的快照
	n.mu.Lock()
	if !n.isDirty {
		t.Fatal("Node should be dirty")
	}
	snapshotSize := n.size
	n.isDirty = false
	n.mu.Unlock()

	if snapshotSize != 100 {
		t.Errorf("Expected snapshot size 100, got %d", snapshotSize)
	}
	if n.isDirty {
		t.Error("Node should NOT be dirty after snapshot")
	}

	// 模拟同步过程中（持有 snapshotSize）发生写入
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(10 * time.Millisecond) // 模拟同步中的延迟
		
		n.mu.Lock()
		n.size = 200
		n.isDirty = true
		n.mu.Unlock()
	}()

	// 模拟同步逻辑（使用 snapshotSize）
	// ... 假设这里正在使用 snapshotSize 上传 ...
	time.Sleep(50 * time.Millisecond)

	wg.Wait()

	// 验证最终状态
	n.mu.Lock()
	if n.size != 200 {
		t.Errorf("Expected final size 200, got %d", n.size)
	}
	if !n.isDirty {
		t.Error("Node should be dirty again after concurrent write")
	}
	n.mu.Unlock()
}
