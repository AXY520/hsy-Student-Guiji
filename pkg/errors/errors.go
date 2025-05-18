package errors

import (
	"fmt"
	"net/http"
)

// ErrorResponse 定义统一的错误响应结构
type ErrorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

// Error 实现error接口
func (e ErrorResponse) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("%s: %s", e.Message, e.Detail)
	}
	return e.Message
}

// ErrorCode 定义错误码类型
type ErrorCode int

const (
	// 系统级错误码 (1000-1999)
	ErrInternal ErrorCode = 1000 + iota
	ErrDatabase
	ErrFileSystem

	// 业务级错误码 (2000-2999)
	ErrInvalidParam ErrorCode = 2000 + iota
	ErrMarkerNotFound
	ErrImageNotFound
	ErrUploadFailed

	// 文件相关错误码 (3000-3999)
	ErrInvalidFileType ErrorCode = 3000 + iota
	ErrFileTooLarge
	ErrFileNameConflict
)

// 错误码与HTTP状态码的映射
var httpStatusMap = map[ErrorCode]int{
	ErrInternal:         http.StatusInternalServerError,
	ErrDatabase:         http.StatusInternalServerError,
	ErrFileSystem:       http.StatusInternalServerError,
	ErrInvalidParam:     http.StatusBadRequest,
	ErrMarkerNotFound:   http.StatusNotFound,
	ErrImageNotFound:    http.StatusNotFound,
	ErrUploadFailed:     http.StatusInternalServerError,
	ErrInvalidFileType:  http.StatusBadRequest,
	ErrFileTooLarge:     http.StatusBadRequest,
	ErrFileNameConflict: http.StatusConflict,
}

// 错误码对应的错误信息
var errorMessages = map[ErrorCode]string{
	ErrInternal:         "内部服务器错误",
	ErrDatabase:         "数据库操作失败",
	ErrFileSystem:       "文件系统操作失败",
	ErrInvalidParam:     "无效的参数",
	ErrMarkerNotFound:   "标记点不存在",
	ErrImageNotFound:    "图片不存在",
	ErrUploadFailed:     "文件上传失败",
	ErrInvalidFileType:  "不支持的文件类型",
	ErrFileTooLarge:     "文件大小超出限制",
	ErrFileNameConflict: "文件名冲突",
}

// NewError 创建新的错误响应
func NewError(code ErrorCode, detail string) error {
	return ErrorResponse{
		Code:    int(code),
		Message: errorMessages[code],
		Detail:  detail,
	}
}

// GetHTTPStatus 获取对应的HTTP状态码
func GetHTTPStatus(code ErrorCode) int {
	if status, ok := httpStatusMap[code]; ok {
		return status
	}
	return http.StatusInternalServerError
}
