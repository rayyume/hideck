package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yibaiba/hideck/internal/config"
	"github.com/yibaiba/hideck/pkg/logger"
)

func stopManagedDevice(c *gin.Context, id string, abandon func(string) error) bool {
	if abandon == nil {
		return true
	}
	if err := abandon(id); err != nil {
		logger.Warn("停止设备失败，保留设备配置", "device", id, "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error", "message": "停止设备失败，配置未修改；请检查设备日志后重试",
		})
		return false
	}
	return true
}

func deleteManagedDevice(c *gin.Context, configPath string, abandon func(string) error) {
	id := deviceIDParam(c)
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "必须填写 id"})
		return
	}
	if !stopManagedDevice(c, id, abandon) {
		return
	}
	if err := config.DeleteDeviceInFile(configPath, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "删除设备配置失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
