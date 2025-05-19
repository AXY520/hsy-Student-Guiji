package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"

	"mapproject/pkg/config"
	"mapproject/pkg/logger"
)

type Marker struct {
	ID          int      `json:"id"`
	Latitude    float64  `json:"latitude"`
	Longitude   float64  `json:"longitude"`
	Description string   `json:"description"`
	Images      []string `json:"images"`
}

func initDB(dbPath string) *sql.DB {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		logger.Log.Fatal("数据库连接失败",
			zap.Error(err),
			zap.String("path", dbPath))
	}

	sqlStmt := `
	CREATE TABLE IF NOT EXISTS markers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		latitude REAL,
		longitude REAL,
		description TEXT
	);
	CREATE TABLE IF NOT EXISTS images (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		marker_id INTEGER,
		filename TEXT,
		FOREIGN KEY(marker_id) REFERENCES markers(id)
	);`

	if _, err = db.Exec(sqlStmt); err != nil {
		logger.Log.Fatal("数据库初始化失败",
			zap.Error(err))
	}

	return db
}

func main() {
	// 加载配置
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 初始化日志
	if err := logger.InitLogger(cfg.Logging.File, cfg.Logging.Level); err != nil {
		log.Fatalf("初始化日志失败: %v", err)
	}

	// 设置 Gin 模式
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	// 初始化数据库
	db := initDB(cfg.Database.Path)
	defer db.Close()

	// 创建上传文件存储目录
	if err := os.MkdirAll(cfg.Server.UploadDir, 0755); err != nil {
		logger.Log.Fatal("创建上传目录失败",
			zap.Error(err),
			zap.String("path", cfg.Server.UploadDir))
	}

	// 静态文件服务
	r.Static("/uploads", cfg.Server.UploadDir)
	r.Static("/static", "./static")

	// API路由
	r.POST("/api/markers", func(c *gin.Context) {
		var marker Marker
		if err := c.ShouldBindJSON(&marker); err != nil {
			logger.Log.Error("解析请求数据失败",
				zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		result, err := db.Exec("INSERT INTO markers (latitude, longitude, description) VALUES (?, ?, ?)",
			marker.Latitude, marker.Longitude, marker.Description)
		if err != nil {
			logger.Log.Error("插入标记点失败",
				zap.Error(err),
				zap.Float64("latitude", marker.Latitude),
				zap.Float64("longitude", marker.Longitude))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		id, _ := result.LastInsertId()
		marker.ID = int(id)

		logger.Log.Info("新增标记点",
			zap.Int("id", marker.ID),
			zap.Float64("latitude", marker.Latitude),
			zap.Float64("longitude", marker.Longitude))

		c.JSON(http.StatusOK, marker)
	})

	// 更新标记点
	r.PUT("/api/markers/:id", func(c *gin.Context) {
		id := c.Param("id")
		var marker Marker
		if err := c.ShouldBindJSON(&marker); err != nil {
			logger.Log.Error("解析请求数据失败",
				zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		result, err := db.Exec("UPDATE markers SET latitude = ?, longitude = ?, description = ? WHERE id = ?",
			marker.Latitude, marker.Longitude, marker.Description, id)
		if err != nil {
			logger.Log.Error("更新标记点失败",
				zap.Error(err),
				zap.String("id", id),
				zap.Float64("latitude", marker.Latitude),
				zap.Float64("longitude", marker.Longitude))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			logger.Log.Error("获取更新结果失败", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		if rowsAffected == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "标记点不存在"})
			return
		}

		logger.Log.Info("更新标记点",
			zap.String("id", id),
			zap.Float64("latitude", marker.Latitude),
			zap.Float64("longitude", marker.Longitude))

		c.JSON(http.StatusOK, marker)
	})

	r.DELETE("/api/markers/:id", func(c *gin.Context) {
		id := c.Param("id")

		// 首先删除关联的图片文件
		rows, err := db.Query("SELECT filename FROM images WHERE marker_id = ?", id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer rows.Close()

		for rows.Next() {
			var filename string
			if err := rows.Scan(&filename); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			// 删除图片文件
			os.Remove(filepath.Join("uploads", filename))
		}

		// 删除数据库中的图片记录
		_, err = db.Exec("DELETE FROM images WHERE marker_id = ?", id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// 删除标记点位
		_, err = db.Exec("DELETE FROM markers WHERE id = ?", id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
	})

	r.POST("/api/markers/:id/images", func(c *gin.Context) {
		markerID := c.Param("id")
		form, err := c.MultipartForm()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		files := form.File["images"]
		var filenames []string

		for _, file := range files {
			filename := filepath.Join("uploads", file.Filename)
			if err := c.SaveUploadedFile(file, filename); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			_, err = db.Exec("INSERT INTO images (marker_id, filename) VALUES (?, ?)",
				markerID, file.Filename)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			filenames = append(filenames, file.Filename)
		}

		c.JSON(http.StatusOK, gin.H{"files": filenames})
	})

	// 删除单个图片
	r.DELETE("/api/markers/:id/images/:filename", func(c *gin.Context) {
		markerID := c.Param("id")
		filename := c.Param("filename")

		// 验证图片是否属于该标记点
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM images WHERE marker_id = ? AND filename = ?",
			markerID, filename).Scan(&count)
		if err != nil {
			logger.Log.Error("验证图片所属关系失败",
				zap.Error(err),
				zap.String("marker_id", markerID),
				zap.String("filename", filename))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		if count == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "图片不存在或不属于该标记点"})
			return
		}

		// 删除数据库记录
		_, err = db.Exec("DELETE FROM images WHERE marker_id = ? AND filename = ?",
			markerID, filename)
		if err != nil {
			logger.Log.Error("删除图片记录失败",
				zap.Error(err),
				zap.String("marker_id", markerID),
				zap.String("filename", filename))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// 删除文件
		err = os.Remove(filepath.Join("uploads", filename))
		if err != nil && !os.IsNotExist(err) {
			logger.Log.Error("删除图片文件失败",
				zap.Error(err),
				zap.String("filename", filename))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		logger.Log.Info("删除图片成功",
			zap.String("marker_id", markerID),
			zap.String("filename", filename))

		c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
	})

	r.GET("/api/markers", func(c *gin.Context) {
		rows, err := db.Query(`
			SELECT m.id, m.latitude, m.longitude, m.description, GROUP_CONCAT(i.filename)
			FROM markers m
			LEFT JOIN images i ON m.id = i.marker_id
			GROUP BY m.id`)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer rows.Close()

		var markers []Marker
		for rows.Next() {
			var marker Marker
			var imagesStr sql.NullString
			err := rows.Scan(&marker.ID, &marker.Latitude, &marker.Longitude, &marker.Description, &imagesStr)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			if imagesStr.Valid && imagesStr.String != "" {
				marker.Images = filepath.SplitList(imagesStr.String)
			}
			markers = append(markers, marker)
		}

		c.JSON(http.StatusOK, markers)
	})

	// HTML页面路由
	r.GET("/admin", func(c *gin.Context) {
		c.File("./static/index.html")
	})

	r.GET("/view", func(c *gin.Context) {
		c.File("./static/view.html")
	})

	r.Run(fmt.Sprintf(":%d", cfg.Server.Port))
}
