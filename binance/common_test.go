package binance

import (
	"fmt"
	"github.com/banbox/banexg/utils"
	"github.com/h2non/gock"
	"strings"
)

type GockItem struct {
	Method  string `json:"method"`
	URL     string `json:"url"`
	Status  int    `json:"status"`
	RspType string `json:"rsp_type"`
	RspPath string `json:"rsp_path"`
}

/*
LoadGockItems
读取并设置需要mock的接口，这里只存储最常用的接口，如markets等
*/
func LoadGockItems(path string) error {
	var items = make([]GockItem, 0)
	err := utils.ReadJsonFile(path, &items)
	if err != nil {
		return err
	}
	for i, item := range items {
		if item.URL == "" {
			return fmt.Errorf("url is required for %d item", i+1)
		}
		if item.RspPath == "" {
			return fmt.Errorf("rsp_path is required for %d item", i+1)
		}
		idx := strings.Index(item.URL, ".")
		subIdx := strings.Index(item.URL[idx:], "/")
		p := item.URL[idx+subIdx:]
		domain := item.URL[:idx+subIdx]
		req := gock.New(domain)
		method := strings.ToLower(item.Method)
		if method == "" {
			method = "get"
		}
		switch method {
		case "get":
			req = req.Get(p)
		case "post":
			req = req.Post(p)
		case "put":
			req = req.Put(p)
		case "delete":
			req = req.Delete(p)
		default:
			return fmt.Errorf("invalid gock method: %s", method)
		}
		if item.Status == 0 {
			item.Status = 200
		}
		rsp := req.Reply(item.Status)
		if item.RspType != "" {
			rsp = rsp.Type(item.RspType)
		}
		rsp.File("testdata/" + item.RspPath)
	}
	return nil
}
