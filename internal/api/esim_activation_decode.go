package api

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yibaiba/hideck/internal/esim"
)

const maxActivationUploadBytes = 12 << 20

func (s *Server) handleEsimDecodeActivation(c *gin.Context) {
	id := deviceIDParam(c)
	if s.pool.GetWorker(id) == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "设备未找到"})
		return
	}
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请上传二维码图片或 PDF"})
		return
	}
	if file.Size > maxActivationUploadBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件太大，请换一张截图或更小的 PDF"})
		return
	}
	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无法读取上传文件"})
		return
	}
	defer src.Close()
	data, err := io.ReadAll(io.LimitReader(src, maxActivationUploadBytes+1))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无法读取上传文件"})
		return
	}
	decoded, err := esim.DecodeActivationMedia(data, file.Filename, file.Header.Get("Content-Type"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, decoded)
}
