package upload

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"mapproject/pkg/errors"
)

// 支持的图片类型
var allowedTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
}

// FileHandler 文件处理器
type FileHandler struct {
	UploadDir   string
	MaxFileSize int64
}

// NewFileHandler 创建新的文件处理器
func NewFileHandler(uploadDir string, maxFileSize int64) *FileHandler {
	return &FileHandler{
		UploadDir:   uploadDir,
		MaxFileSize: maxFileSize,
	}
}

// SaveFile 保存上传的文件
func (h *FileHandler) SaveFile(file *multipart.FileHeader) (string, error) {
	// 检查文件大小
	if file.Size > h.MaxFileSize {
		return "", errors.NewError(errors.ErrFileTooLarge,
			fmt.Sprintf("文件大小超过限制：%d bytes", h.MaxFileSize))
	}

	// 打开文件
	src, err := file.Open()
	if err != nil {
		return "", errors.NewError(errors.ErrUploadFailed, err.Error())
	}
	defer src.Close()

	// 读取文件头部以检查文件类型
	buffer := make([]byte, 512)
	_, err = src.Read(buffer)
	if err != nil {
		return "", errors.NewError(errors.ErrUploadFailed, err.Error())
	}

	// 重置文件指针
	src.Seek(0, 0)

	fileType := http.DetectContentType(buffer)
	if !allowedTypes[fileType] {
		return "", errors.NewError(errors.ErrInvalidFileType,
			fmt.Sprintf("不支持的文件类型：%s", fileType))
	}

	// 生成唯一文件名
	ext := filepath.Ext(file.Filename)
	hash := md5.New()
	timeStr := time.Now().Format("20060102150405")
	hash.Write([]byte(file.Filename + timeStr))
	filename := hex.EncodeToString(hash.Sum(nil))[:16] + ext

	// 确保上传目录存在
	if err := os.MkdirAll(h.UploadDir, 0755); err != nil {
		return "", errors.NewError(errors.ErrFileSystem, err.Error())
	}

	// 创建目标文件
	dst, err := os.Create(filepath.Join(h.UploadDir, filename))
	if err != nil {
		return "", errors.NewError(errors.ErrFileSystem, err.Error())
	}
	defer dst.Close()

	// 复制文件内容
	if _, err = io.Copy(dst, src); err != nil {
		return "", errors.NewError(errors.ErrUploadFailed, err.Error())
	}

	return filename, nil
}

// DeleteFile 删除文件
func (h *FileHandler) DeleteFile(filename string) error {
	if err := os.Remove(filepath.Join(h.UploadDir, filename)); err != nil {
		if os.IsNotExist(err) {
			return errors.NewError(errors.ErrImageNotFound, err.Error())
		}
		return errors.NewError(errors.ErrFileSystem, err.Error())
	}
	return nil
}
