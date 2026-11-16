package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"squirrel-downloader/internal/model"
	"squirrel-downloader/internal/service"

	"github.com/gin-gonic/gin"
)

// CreateTaskRequest 创建任务请求
type CreateTaskRequest struct {
	URL                string `json:"url" binding:"required"`
	OutputName         string `json:"output_name"` // 允许为空：service 层会用 generateOutputName(url) 兜底
	ThreadCount        int    `json:"thread_count"`
	RetryCount         int    `json:"retry_count"`
	Headers            string `json:"headers"`
	BaseURL            string `json:"base_url"`
	DelAfterDone       *bool  `json:"del_after_done"`
	BinaryMerge        bool   `json:"binary_merge"`
	AutoSelect         *bool  `json:"auto_select"` // 缺省按 true 处理，见 CreateTask 中的默认值
	SkipSegmentsCheck  bool   `json:"skip_segments_check"`
	ConcurrentDownload bool   `json:"concurrent_download"`
	Key                string `json:"key"`
	DecryptionEngine   string `json:"decryption_engine"`
	CustomArgs         string `json:"custom_args"`
	CustomProxy        string `json:"custom_proxy"`
}

func ListTasks(c *gin.Context) {
	var tasks []model.Task
	status := c.Query("status")

	query := model.GetDB().Model(&model.Task{})
	// 支持逗号分隔的多状态筛选，例如 status=failed,interrupted
	if status != "" {
		query = query.Where("status IN ?", strings.Split(status, ","))
	}
	query = query.Order("created_at DESC").Find(&tasks)

	c.JSON(http.StatusOK, tasks)
}

func CreateTask(c *gin.Context) {
	var req CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}

	task, err := createOneTask(&req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, task)
}

// createOneTask 把请求里的默认值和 service 调用收在一处，
// 单条创建与批量创建共用，避免两边逻辑漂移。
func createOneTask(req *CreateTaskRequest) (*model.Task, error) {
	if strings.TrimSpace(req.URL) == "" {
		return nil, errEmptyURL
	}

	// 设置默认值
	if req.ThreadCount <= 0 {
		req.ThreadCount = 32
	}
	if req.RetryCount <= 0 {
		req.RetryCount = 15
	}
	if req.DecryptionEngine == "" {
		req.DecryptionEngine = "MP4DECRYPT"
	}

	// auto_select 缺省为 true。
	// N_m3u8DL-RE 解析到多码率 master playlist 时会弹出交互式选流菜单；容器内没有
	// TTY，Spectre.Console 读不到按键会抛 InvalidOperationException，任务在 0% 中断。
	// 只有显式传 -sv best -sa best 才能绕过该菜单，因此缺省必须为 true。
	autoSelect := true
	if req.AutoSelect != nil {
		autoSelect = *req.AutoSelect
	}

	// 转换为 service 层的请求类型
	serviceReq := &service.CreateTaskRequest{
		URL:                req.URL,
		OutputName:         req.OutputName,
		ThreadCount:        req.ThreadCount,
		RetryCount:         req.RetryCount,
		Headers:            req.Headers,
		BaseURL:            req.BaseURL,
		DelAfterDone:       req.DelAfterDone,
		BinaryMerge:        req.BinaryMerge,
		AutoSelect:         autoSelect,
		SkipSegmentsCheck:  req.SkipSegmentsCheck,
		ConcurrentDownload: req.ConcurrentDownload,
		Key:                req.Key,
		DecryptionEngine:   req.DecryptionEngine,
		CustomArgs:         req.CustomArgs,
		CustomProxy:        req.CustomProxy,
	}

	return service.CreateTask(serviceReq)
}

func GetTask(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的任务ID"})
		return
	}

	task, err := service.GetTaskByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		return
	}

	c.JSON(http.StatusOK, task)
}

func DeleteTask(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的任务ID"})
		return
	}

	if err := service.DeleteTask(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

// CancelTask 取消正在进行中的任务（保留记录与参数，便于重试）
func CancelTask(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的任务ID"})
		return
	}

	task, err := service.CancelTask(uint(id))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, task)
}

// RetryTask 重试失败/中断的任务：重置为等待中，由轮询重新开始下载
func RetryTask(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的任务ID"})
		return
	}

	task, err := service.RetryTask(uint(id))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, task)
}

type BatchDeleteTasksRequest struct {
	IDs []uint `json:"ids" binding:"required,min=1"`
}

// BatchDeleteTasks 批量删除任务。
// 使用 DELETE /tasks + JSON body，避免与 /tasks/:id 产生路由冲突。
func BatchDeleteTasks(c *gin.Context) {
	var req BatchDeleteTasksRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}

	deleted, failed := service.DeleteTasks(req.IDs)

	c.JSON(http.StatusOK, gin.H{
		"deleted": deleted,
		"failed":  failed,
	})
}

func GetTaskLog(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的任务ID"})
		return
	}

	logContent, err := service.GetTaskLog(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"log": logContent})
}

// errEmptyURL 用于区分「参数不合法」和「服务端错误」。
var errEmptyURL = errors.New("url 不能为空")

// BatchCreateTaskRequest 批量创建请求。
// 同时兼容两种写法：裸数组 [...] 或 {"tasks": [...]}。
type BatchCreateTaskRequest struct {
	Tasks []CreateTaskRequest `json:"tasks"`
}

// BatchCreateError 单条失败的原因，下标对应请求里的位置。
type BatchCreateError struct {
	Index int    `json:"index"`
	URL   string `json:"url"`
	Error string `json:"error"`
}

// BatchCreateResult 批量创建结果。
// Created 与 Errors 一一对应请求中的任务，按下标即可定位。
type BatchCreateResult struct {
	Created []*model.Task      `json:"created"`
	Errors  []BatchCreateError `json:"errors"`
}

// 单次批量提交的上限，防止一次塞几千条把队列撑爆。
const maxBatchTasks = 50

// BatchCreateTasks 批量创建下载任务。
//
// 部分失败不会导致整体失败：成功的任务照常入队，失败的按下标返回原因。
// Agent 可以据此只重提交失败的那几条。
func BatchCreateTasks(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "读取请求体失败"})
		return
	}

	var reqs []CreateTaskRequest
	trimmed := strings.TrimSpace(string(body))
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal(body, &reqs); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
			return
		}
	} else {
		var wrapper BatchCreateTaskRequest
		if err := json.Unmarshal(body, &wrapper); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
			return
		}
		reqs = wrapper.Tasks
	}

	if len(reqs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tasks 不能为空"})
		return
	}
	if len(reqs) > maxBatchTasks {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("一次最多提交 %d 个任务，本次 %d 个", maxBatchTasks, len(reqs)),
		})
		return
	}

	result := BatchCreateResult{
		Created: make([]*model.Task, 0, len(reqs)),
		Errors:  make([]BatchCreateError, 0),
	}

	for i := range reqs {
		task, err := createOneTask(&reqs[i])
		if err != nil {
			result.Errors = append(result.Errors, BatchCreateError{
				Index: i,
				URL:   reqs[i].URL,
				Error: err.Error(),
			})
			continue
		}
		result.Created = append(result.Created, task)
	}

	// 全部失败才返回 400，只要有一条成功就是 200
	status := http.StatusOK
	if len(result.Created) == 0 {
		status = http.StatusBadRequest
	}
	c.JSON(status, result)
}
