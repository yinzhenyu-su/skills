package quark

import (
	"fmt"
	"net/http"
)

type ManageService struct {
	client *Client
}

func NewManageService(client *Client) *ManageService {
	return &ManageService{client: client}
}

func (s *ManageService) CreateDir(pdirFid, name string) (string, error) {
	data := map[string]interface{}{
		"pdir_fid":      pdirFid,
		"file_name":     name,
		"dir_path":      "",
		"dir_init_lock": false,
	}
	var resp CreateDirResp
	err := s.client.Request(http.MethodPost, "/file", nil, data, &resp)
	if err != nil {
		return "", err
	}
	if resp.Status >= 400 || resp.Code != 0 {
		return "", fmt.Errorf("CreateDir error: status=%d, code=%d, message=%s", resp.Status, resp.Code, resp.Message)
	}
	return resp.Data.Fid, nil
}

func (s *ManageService) Delete(fids []string) error {
	data := map[string]interface{}{
		"action_type":  1,
		"exclude_fids": []string{},
		"filelist":     fids,
	}
	var resp Resp
	err := s.client.Request(http.MethodPost, "/file/delete", nil, data, &resp)
	if err != nil {
		return err
	}
	if resp.Status >= 400 || resp.Code != 0 {
		return fmt.Errorf("Delete error: status=%d, code=%d, message=%s", resp.Status, resp.Code, resp.Message)
	}
	return nil
}

func (s *ManageService) Rename(fid, newName string) error {
	data := map[string]interface{}{
		"fid":       fid,
		"file_name": newName,
	}
	var resp Resp
	err := s.client.Request(http.MethodPost, "/file/rename", nil, data, &resp)
	if err != nil {
		return err
	}
	if resp.Status >= 400 || resp.Code != 0 {
		return fmt.Errorf("Rename error: status=%d, code=%d, message=%s", resp.Status, resp.Code, resp.Message)
	}
	return nil
}

func (s *ManageService) Move(fids []string, toPdirFid string, currentDirFid string) error {
	data := map[string]interface{}{
		"filelist":     fids,
		"to_pdir_fid":  toPdirFid,
		"action_type":  1,
		"exclude_fids": []string{},
	}
	var resp Resp
	err := s.client.Request(http.MethodPost, "/file/move", nil, data, &resp)
	if err != nil {
		return err
	}
	if resp.Status >= 400 || resp.Code != 0 {
		return fmt.Errorf("Move error: status=%d, code=%d, message=%s", resp.Status, resp.Code, resp.Message)
	}
	return nil
}
