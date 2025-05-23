package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"mapproject/pkg/config"
	"mapproject/pkg/logger"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
)

type Marker struct {
	ID            int      `json:"id"`
	Latitude      float64  `json:"latitude"`
	Longitude     float64  `json:"longitude"`
	Value         float64  `json:"value"`
	RequiredValue float64  `json:"required_value"`
	Images        []string `json:"images"`
}

type App struct {
	DB     *sql.DB
	Cfg    *config.Config
	Logger *zap.Logger
}

func initDB(dbPath string) *sql.DB {
	db, err := sql.Open("sqlite3", dbPath+"?_busy_timeout=5000")
	if err != nil {
		logger.Log.Fatal("数据库连接失败", zap.Error(err), zap.String("path", dbPath))
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	// 检查schema_version表是否存在
	var tableExists int
	err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_version'").Scan(&tableExists)
	if err != nil {
		logger.Log.Fatal("检查schema_version表失败", zap.Error(err))
	}

	if tableExists == 0 {
		// 初始化schema_version表
		if _, err := db.Exec("CREATE TABLE schema_version (id INTEGER PRIMARY KEY, version INTEGER NOT NULL DEFAULT 1)"); err != nil {
			logger.Log.Fatal("创建schema_version表失败", zap.Error(err))
		}
		if _, err := db.Exec("INSERT INTO schema_version (id, version) VALUES (1, 1)"); err != nil {
			logger.Log.Fatal("初始化schema_version失败", zap.Error(err))
		}
	}

	// 获取当前版本
	var version int
	err = db.QueryRow("SELECT version FROM schema_version WHERE id = 1").Scan(&version)
	if err != nil {
		logger.Log.Fatal("获取schema版本失败", zap.Error(err))
	}

	if version < 2 {
		// 执行迁移
		migrateToVersion2(db)
		_, err = db.Exec("UPDATE schema_version SET version = 2 WHERE id = 1")
		if err != nil {
			logger.Log.Fatal("更新 schema 版本失败", zap.Error(err))
		}
	}

	return db
}

func createTables(db *sql.DB) {
	sqlStmt := `
	CREATE TABLE IF NOT EXISTS schema_version (
		id INTEGER PRIMARY KEY,
		version INTEGER NOT NULL DEFAULT 1
	);
	CREATE TABLE IF NOT EXISTS markers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		latitude REAL,
		longitude REAL,
		value REAL DEFAULT 0,
		required_value REAL DEFAULT 0
	);
	CREATE TABLE IF NOT EXISTS images (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		marker_id INTEGER,
		filename TEXT,
		FOREIGN KEY(marker_id) REFERENCES markers(id)
	);`
	if _, err := db.Exec(sqlStmt); err != nil {
		logger.Log.Fatal("创建表失败", zap.Error(err))
	}
}

func migrateToVersion2(db *sql.DB) {
	// 检查是否存在旧表
	var hasOldTable bool
	err := db.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master 
		WHERE type='table' AND name='markers' 
		AND sql LIKE '%description%'`).Scan(&hasOldTable)
	if err != nil {
		logger.Log.Fatal("检查旧表结构失败", zap.Error(err))
	}

	if hasOldTable {
		// 备份旧数据
		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS markers_backup AS SELECT * FROM markers;`)
		if err != nil {
			logger.Log.Fatal("备份数据失败", zap.Error(err))
		}
		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS images_backup AS SELECT * FROM images;`)
		if err != nil {
			logger.Log.Fatal("备份图片数据失败", zap.Error(err))
		}

		// 删除旧表
		_, err = db.Exec(`DROP TABLE markers; DROP TABLE images;`)
		if err != nil {
			logger.Log.Fatal("删除旧表失败", zap.Error(err))
		}

		// 重新创建表
		createTables(db)

		// 迁移数据
		_, err = db.Exec(`
			INSERT INTO markers (id, latitude, longitude, value, required_value)
			SELECT id, latitude, longitude, 0, 0 FROM markers_backup;
		`)
		if err != nil {
			logger.Log.Fatal("迁移数据失败", zap.Error(err))
		}
		_, err = db.Exec(`
			INSERT INTO images (marker_id, filename)
			SELECT marker_id, filename FROM images_backup;
		`)
		if err != nil {
			logger.Log.Fatal("迁移图片数据失败", zap.Error(err))
		}

		// 删除备份表
		_, err = db.Exec(`DROP TABLE markers_backup; DROP TABLE images_backup;`)
		if err != nil {
			logger.Log.Fatal("删除备份表失败", zap.Error(err))
		}
	}
}

func errorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) > 0 {
			err := c.Errors.Last()
			logger.Log.Error("请求处理失败", zap.Error(err), zap.String("request_id", c.GetString("request_id")))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
	}
}

func requestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := fmt.Sprintf("%d", time.Now().UnixNano())
		c.Set("request_id", requestID)
		logger.Log = logger.Log.With(zap.String("request_id", requestID))
		c.Next()
	}
}

func (app *App) CreateMarker(c *gin.Context) {
	var marker Marker
	if err := c.ShouldBindJSON(&marker); err != nil {
		app.Logger.Error("解析请求数据失败", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := app.DB.Exec("INSERT INTO markers (latitude, longitude, value, required_value) VALUES (?, ?, ?, ?)",
		marker.Latitude, marker.Longitude, marker.Value, marker.RequiredValue)
	if err != nil {
		app.Logger.Error("插入标记点失败",
			zap.Error(err),
			zap.Float64("latitude", marker.Latitude),
			zap.Float64("longitude", marker.Longitude))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	id, _ := result.LastInsertId()
	marker.ID = int(id)

	app.Logger.Info("新增标记点",
		zap.Int("id", marker.ID),
		zap.Float64("latitude", marker.Latitude),
		zap.Float64("longitude", marker.Longitude))

	c.JSON(http.StatusOK, marker)
}

func (app *App) UpdateMarker(c *gin.Context) {
	id := c.Param("id")
	var marker Marker
	if err := c.ShouldBindJSON(&marker); err != nil {
		app.Logger.Error("解析请求数据失败", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := app.DB.Exec("UPDATE markers SET latitude = ?, longitude = ?, value = ?, required_value = ? WHERE id = ?",
		marker.Latitude, marker.Longitude, marker.Value, marker.RequiredValue, id)
	if err != nil {
		app.Logger.Error("更新标记点失败",
			zap.Error(err),
			zap.String("id", id),
			zap.Float64("latitude", marker.Latitude),
			zap.Float64("longitude", marker.Longitude))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		app.Logger.Error("获取更新结果失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "标记点不存在"})
		return
	}

	app.Logger.Info("更新标记点",
		zap.String("id", id),
		zap.Float64("latitude", marker.Latitude),
		zap.Float64("longitude", marker.Longitude))

	c.JSON(http.StatusOK, marker)
}

func (app *App) DeleteMarker(c *gin.Context) {
	id := c.Param("id")
	tx, err := app.DB.Begin()
	if err != nil {
		app.Logger.Error("开始事务失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer tx.Rollback()

	rows, err := tx.Query("SELECT filename FROM images WHERE marker_id = ?", id)
	if err != nil {
		app.Logger.Error("查询图片失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	for rows.Next() {
		var filename string
		if err := rows.Scan(&filename); err != nil {
			app.Logger.Error("扫描图片数据失败", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		os.Remove(filepath.Join(app.Cfg.Server.UploadDir, filepath.Base(filename)))
	}

	if _, err := tx.Exec("DELETE FROM images WHERE marker_id = ?", id); err != nil {
		app.Logger.Error("删除图片记录失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if _, err := tx.Exec("DELETE FROM markers WHERE id = ?", id); err != nil {
		app.Logger.Error("删除标记点失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := tx.Commit(); err != nil {
		app.Logger.Error("提交事务失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

func (app *App) UploadImages(c *gin.Context) {
	markerID := c.Param("id")
	form, err := c.MultipartForm()
	if err != nil {
		app.Logger.Error("解析表单数据失败", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	files := form.File["images"]
	var filenames []string
	maxSize := int64(5 << 20) // 5MB
	allowedTypes := map[string]bool{
		"image/jpeg": true,
		"image/png":  true,
	}

	for _, file := range files {
		if file.Size > maxSize {
			app.Logger.Error("文件过大", zap.String("filename", file.Filename))
			c.JSON(http.StatusBadRequest, gin.H{"error": "文件过大"})
			return
		}
		if !allowedTypes[file.Header.Get("Content-Type")] {
			app.Logger.Error("不支持的文件类型", zap.String("filename", file.Filename))
			c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的文件类型"})
			return
		}

		filename := filepath.Base(file.Filename)
		savePath := filepath.Join(app.Cfg.Server.UploadDir, filename)
		if err := c.SaveUploadedFile(file, savePath); err != nil {
			app.Logger.Error("保存文件失败", zap.Error(err), zap.String("filename", filename))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		_, err = app.DB.Exec("INSERT INTO images (marker_id, filename) VALUES (?, ?)",
			markerID, filename)
		if err != nil {
			app.Logger.Error("插入图片记录失败", zap.Error(err), zap.String("filename", filename))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		filenames = append(filenames, filename)
	}

	c.JSON(http.StatusOK, gin.H{"files": filenames})
}

func (app *App) DeleteImage(c *gin.Context) {
	markerID := c.Param("id")
	filename := c.Param("filename")

	var count int
	err := app.DB.QueryRow("SELECT COUNT(*) FROM images WHERE marker_id = ? AND filename = ?",
		markerID, filename).Scan(&count)
	if err != nil {
		app.Logger.Error("验证图片所属关系失败",
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

	tx, err := app.DB.Begin()
	if err != nil {
		app.Logger.Error("开始事务失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM images WHERE marker_id = ? AND filename = ?",
		markerID, filename); err != nil {
		app.Logger.Error("删除图片记录失败",
			zap.Error(err),
			zap.String("marker_id", markerID),
			zap.String("filename", filename))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	filename = filepath.Base(filename)
	err = os.Remove(filepath.Join(app.Cfg.Server.UploadDir, filename))
	if err != nil && !os.IsNotExist(err) {
		app.Logger.Error("删除图片文件失败",
			zap.Error(err),
			zap.String("filename", filename))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := tx.Commit(); err != nil {
		app.Logger.Error("提交事务失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	app.Logger.Info("删除图片成功",
		zap.String("marker_id", markerID),
		zap.String("filename", filename))

	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

func (app *App) GetMarkers(c *gin.Context) {
	rows, err := app.DB.Query("SELECT id, latitude, longitude, value, required_value FROM markers")
	if err != nil {
		app.Logger.Error("查询标记点失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	markers := make(map[int]*Marker)
	for rows.Next() {
		var m Marker
		if err := rows.Scan(&m.ID, &m.Latitude, &m.Longitude, &m.Value, &m.RequiredValue); err != nil {
			app.Logger.Error("扫描标记点数据失败", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		markers[m.ID] = &m
	}

	rows, err = app.DB.Query("SELECT marker_id, filename FROM images")
	if err != nil {
		app.Logger.Error("查询图片失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	for rows.Next() {
		var markerID int
		var filename string
		if err := rows.Scan(&markerID, &filename); err != nil {
			app.Logger.Error("扫描图片数据失败", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if m, ok := markers[markerID]; ok {
			m.Images = append(m.Images, filename)
		}
	}

	var result []Marker
	for _, m := range markers {
		result = append(result, *m)
	}

	c.JSON(http.StatusOK, result)
}

func main() {
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	if err := logger.InitLogger(cfg.Logging.File, cfg.Logging.Level); err != nil {
		log.Fatalf("初始化日志失败: %v", err)
	}

	if cfg.Server.UploadDir == "" || cfg.Server.Port == 0 {
		log.Fatalf("配置不完整: UploadDir 或 Port 缺失")
	}

	db := initDB(cfg.Database.Path)
	defer db.Close()

	if err := os.MkdirAll(cfg.Server.UploadDir, 0755); err != nil {
		logger.Log.Fatal("创建上传目录失败",
			zap.Error(err),
			zap.String("path", cfg.Server.UploadDir))
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.Use(errorHandler())

	// 自定义静态文件处理
	r.Use(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/uploads/") {
			c.Header("Cache-Control", "public, max-age=3600")
		}
		c.Next()
	})
	r.Static("/uploads", cfg.Server.UploadDir)
	r.Static("/static", "./static")

	app := &App{DB: db, Cfg: cfg, Logger: logger.Log}

	api := r.Group("/api")
	{
		markers := api.Group("/markers")
		{
			markers.POST("", app.CreateMarker)
			markers.GET("", app.GetMarkers)
			markers.PUT("/:id", app.UpdateMarker)
			markers.DELETE("/:id", app.DeleteMarker)
			markers.POST("/:id/images", app.UploadImages)
			markers.DELETE("/:id/images/:filename", app.DeleteImage)
		}
	}

	r.GET("/admin", func(c *gin.Context) {
		c.File("./static/index.html")
	})
	r.GET("/view", func(c *gin.Context) {
		c.File("./static/view.html")
	})
	r.GET("/login", func(c *gin.Context) {
		c.File("./static/login.html")
	})
	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/login")
	})

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: r,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Log.Fatal("服务器启动失败", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Log.Fatal("服务器关闭失败", zap.Error(err))
	}
}

func handleDeleteMarker(c *gin.Context, db *sql.DB, uploadDir string) {
	id := c.Param("id")

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

		safeFilename := filepath.Base(filename)
		filePath := filepath.Join(uploadDir, safeFilename)

		if !strings.HasPrefix(filePath, uploadDir) {
			logger.Log.Warn("非法文件路径",
				zap.String("filename", filename),
				zap.String("resolved", filePath))
			continue
		}

		if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
			logger.Log.Error("删除文件失败",
				zap.String("path", filePath),
				zap.Error(err))
		}
	}

	if _, err := db.Exec("DELETE FROM images WHERE marker_id = ?", id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if _, err := db.Exec("DELETE FROM markers WHERE id = ?", id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}
