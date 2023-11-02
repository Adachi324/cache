package cache

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"
)

type cacheLoader struct {
	Id int64
}

type cacheResp struct {
	Msg        string  `json:"msg"`
	PointerMsg *string `json:"val"`
	Id         int64   `json:"id"`
}

func (loader *cacheLoader) Loader(ctx context.Context, _ []string) ([]interface{}, error) {

	resp, err := loader.loaderFromSource(ctx)
	if err != nil {
		return nil, err
	}
	return []interface{}{resp}, nil
}

func (loader *cacheLoader) loaderFromSource(ctx context.Context) (string, error) {
	unix := time.Now().Unix()
	str := strconv.FormatInt(unix, 10)
	return str, nil
}

func (loader *cacheResp) Loader(ctx context.Context, _ []string) ([]interface{}, error) {

	resp, err := loader.loaderFromSource(ctx)
	if err != nil {
		return nil, err
	}
	return []interface{}{resp}, nil
}

func (loader *cacheResp) loaderFromSource(ctx context.Context) (*cacheResp, error) {
	id := loader.Id
	fmt.Printf("调用loaderFromSource方法时id的值：%v\n", id)
	id = id + 1
	sId := strconv.FormatInt(id, 10)
	resp := &cacheResp{
		Id:         id,
		Msg:        sId,
		PointerMsg: &sId,
	}
	return resp, nil
}

func TestInCacheExpiration(t *testing.T) {
	ctx := context.Background()
	InitUnifiedCache(0, 0)
	unifiedCache := GetUnifiedCache()
	rsp := &cacheResp{}
	key := "123"
	err := unifiedCache.LoadWithDefaultExpiration(ctx, rsp.Loader, key, &rsp)
	if err != nil {
		fmt.Printf("load cache err:%v", err)
	}
	fmt.Printf("第一次外部调用:%v\n", rsp.Id)
	time.Sleep(time.Second)
	err = unifiedCache.LoadWithDefaultExpiration(ctx, rsp.Loader, key, &rsp)
	if err != nil {
		fmt.Printf("load cache err:%v", err)
	}
	fmt.Printf("第二次外部调用(sleep 1s):%v\n", rsp.Id)
	time.Sleep(10 * time.Second)
	err = unifiedCache.LoadWithDefaultExpiration(ctx, rsp.Loader, key, &rsp)
	if err != nil {
		fmt.Printf("load cache err:%v", err)
	}
	fmt.Printf("第三次外部调用(sleep 10s):%v\n", rsp.Id)
	time.Sleep(1 * time.Second)
	err = unifiedCache.LoadWithDefaultExpiration(ctx, rsp.Loader, key, &rsp)
	if err != nil {
		fmt.Printf("load cache err:%v", err)
	}
	fmt.Printf("第四次外部调用(sleep 1s):%v\n", rsp.Id)

}

func TestCacheStruct(*testing.T) {
	InitUnifiedCache(0, 0)
	unifiedCache := GetUnifiedCache()
	var cached *cacheResp
	ctx := context.Background()
	err := unifiedCache.LoadWithExpiration(ctx, cached.Loader, "abc", &cached, 2, 60)
	if err != nil {
		fmt.Println("load err:", err)
	}
	fmt.Printf("1.id:%v\n", cached.Id)
	time.Sleep(2 * time.Second)
	err = unifiedCache.LoadWithExpiration(ctx, cached.Loader, "abc", &cached, 5, 5)
	if err != nil {
		fmt.Println("load err:", err)
	}
	fmt.Printf("2.id:%v\n", cached.Id)
	time.Sleep(2 * time.Second)
	err = unifiedCache.LoadWithExpiration(ctx, cached.Loader, "abc", &cached, 5, 5)
	if err != nil {
		fmt.Println("load err:", err)
	}
	fmt.Printf("3.id:%v\n", cached.Id)
	time.Sleep(6 * time.Second)
	err = unifiedCache.LoadWithExpiration(ctx, cached.Loader, "abc", &cached, 10, 10)
	if err != nil {
		fmt.Println("load err:", err)
	}
	fmt.Printf("4.id:%v\n", cached.Id)

}

func TestNoKeyCache(*testing.T) {
	InitUnifiedCache(0, 0)
	unifiedCache := GetUnifiedCache()
	var cached *cacheResp
	ctx := context.Background()
	err := unifiedCache.LoadWithExpiration(ctx, cached.Loader, "", &cached, 2, 60)
	if err != nil {
		fmt.Println("load err:", err)
	}

}
