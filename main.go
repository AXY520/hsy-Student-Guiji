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
	ID                int      `json:"id"`
	Latitude          float64  `json:"latitude"`
	Longitude         float64  `json:"longitude"`
	Value             float64  `json:"value"`
	RequiredValue     float64  `json:"required_value"`
	Description       string   `json:"description"`
	SufficientColor   string   `json:"sufficient_color"`
	InsufficientColor string   `json:"insufficient_color"`
	Images            []string `json:"images"`
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

	// 创建所有必要的表
	createTables(db)

	// 运行迁移
	migrateDatabase(db)

	return db
}

func createTables(db *sql.DB) {
	sqlStmt := `
	CREATE TABLE IF NOT EXISTS markers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		latitude REAL NOT NULL,
		longitude REAL NOT NULL,
		value REAL DEFAULT 0,
		required_value REAL DEFAULT 0,
		description TEXT DEFAULT '',
		sufficient_color TEXT DEFAULT '#409EFF',
		insufficient_color TEXT DEFAULT '#F56C6C',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS images (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		marker_id INTEGER NOT NULL,
		filename TEXT NOT NULL,
		file_size INTEGER DEFAULT 0,
		mime_type TEXT DEFAULT '',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(marker_id) REFERENCES markers(id) ON DELETE CASCADE
	);
	CREATE TABLE IF NOT EXISTS visits (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ip TEXT,
		user_agent TEXT,
		path TEXT,
		visit_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		referer TEXT
	);
	CREATE TABLE IF NOT EXISTS user_actions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ip TEXT,
		user_agent TEXT,
		action_type TEXT,
		action_detail TEXT,
		target_id TEXT,
		action_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	-- 创建索引
	CREATE INDEX IF NOT EXISTS idx_markers_location ON markers(latitude, longitude);
	CREATE INDEX IF NOT EXISTS idx_markers_created_at ON markers(created_at);
	CREATE INDEX IF NOT EXISTS idx_images_marker_id ON images(marker_id);
	CREATE INDEX IF NOT EXISTS idx_visits_ip ON visits(ip);
	CREATE INDEX IF NOT EXISTS idx_visits_visit_time ON visits(visit_time);
	CREATE INDEX IF NOT EXISTS idx_user_actions_ip ON user_actions(ip);
	CREATE INDEX IF NOT EXISTS idx_user_actions_action_time ON user_actions(action_time);
	CREATE INDEX IF NOT EXISTS idx_user_actions_action_type ON user_actions(action_type);

	-- 创建触发器
	CREATE TRIGGER IF NOT EXISTS update_markers_updated_at
		AFTER UPDATE ON markers
		FOR EACH ROW
		BEGIN
		    UPDATE markers SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
		END;

	-- 创建视图
	CREATE VIEW IF NOT EXISTS marker_summary AS
	SELECT
	    m.id,
	    m.latitude,
	    m.longitude,
	    m.value,
	    m.required_value,
	    m.created_at,
	    m.updated_at,
	    COUNT(i.id) as image_count,
	    CASE
	        WHEN m.value >= m.required_value THEN 'sufficient'
	        ELSE 'insufficient'
	    END as status
	FROM markers m
	LEFT JOIN images i ON m.id = i.marker_id
	GROUP BY m.id;

	CREATE VIEW IF NOT EXISTS visit_stats AS
	SELECT
	    ip,
	    user_agent,
	    COUNT(*) as visit_count,
	    MAX(visit_time) as last_visit,
	    MIN(visit_time) as first_visit,
	    COUNT(DISTINCT path) as unique_paths
	FROM visits
	GROUP BY ip, user_agent;`

	if _, err := db.Exec(sqlStmt); err != nil {
		logger.Log.Fatal("创建表失败", zap.Error(err))
	}
}

func migrateDatabase(db *sql.DB) {
	// 检查 markers 表是否有 description 列
	var hasDescriptionColumn bool
	err := db.QueryRow(`
		SELECT COUNT(*) > 0 
		FROM pragma_table_info('markers') 
		WHERE name = 'description'
	`).Scan(&hasDescriptionColumn)

	if err != nil {
		logger.Log.Error("检查 description 列失败", zap.Error(err))
		return
	}

	// 如果没有 description 列，添加它
	if !hasDescriptionColumn {
		_, err = db.Exec(`ALTER TABLE markers ADD COLUMN description TEXT DEFAULT ''`)
		if err != nil {
			logger.Log.Error("添加 description 列失败", zap.Error(err))
			return
		}
		logger.Log.Info("成功添加 description 列到 markers 表")
	}

	// 检查 markers 表是否有 sufficient_color 列
	var hasSufficientColorColumn bool
	err = db.QueryRow(`
		SELECT COUNT(*) > 0 
		FROM pragma_table_info('markers') 
		WHERE name = 'sufficient_color'
	`).Scan(&hasSufficientColorColumn)

	if err != nil {
		logger.Log.Error("检查 sufficient_color 列失败", zap.Error(err))
		return
	}

	// 检查 markers 表是否有 insufficient_color 列
	var hasInsufficientColorColumn bool
	err = db.QueryRow(`
		SELECT COUNT(*) > 0 
		FROM pragma_table_info('markers') 
		WHERE name = 'insufficient_color'
	`).Scan(&hasInsufficientColorColumn)

	if err != nil {
		logger.Log.Error("检查 insufficient_color 列失败", zap.Error(err))
		return
	}

	// 如果没有 sufficient_color 列，添加它
	if !hasSufficientColorColumn {
		_, err = db.Exec(`ALTER TABLE markers ADD COLUMN sufficient_color TEXT DEFAULT '#409EFF'`)
		if err != nil {
			logger.Log.Error("添加 sufficient_color 列失败", zap.Error(err))
			return
		}
		logger.Log.Info("成功添加 sufficient_color 列到 markers 表")
	}

	// 如果没有 insufficient_color 列，添加它
	if !hasInsufficientColorColumn {
		_, err = db.Exec(`ALTER TABLE markers ADD COLUMN insufficient_color TEXT DEFAULT '#F56C6C'`)
		if err != nil {
			logger.Log.Error("添加 insufficient_color 列失败", zap.Error(err))
			return
		}
		logger.Log.Info("成功添加 insufficient_color 列到 markers 表")
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

func (app *App) CreateMarker(c *gin.Context) {
	var marker Marker
	if err := c.ShouldBindJSON(&marker); err != nil {
		app.Logger.Error("解析请求数据失败", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 设置默认颜色（如果未提供）
	if marker.SufficientColor == "" {
		marker.SufficientColor = "#409EFF"
	}
	if marker.InsufficientColor == "" {
		marker.InsufficientColor = "#F56C6C"
	}

	result, err := app.DB.Exec("INSERT INTO markers (latitude, longitude, value, required_value, description, sufficient_color, insufficient_color) VALUES (?, ?, ?, ?, ?, ?, ?)",
		marker.Latitude, marker.Longitude, marker.Value, marker.RequiredValue, marker.Description, marker.SufficientColor, marker.InsufficientColor)
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

	// 记录创建标记点的操作
	app.recordUserAction(c, "create_marker",
		fmt.Sprintf("创建标记点 (%.6f, %.6f)", marker.Latitude, marker.Longitude),
		fmt.Sprintf("%d", marker.ID))

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

	// 设置默认颜色（如果未提供）
	if marker.SufficientColor == "" {
		marker.SufficientColor = "#409EFF"
	}
	if marker.InsufficientColor == "" {
		marker.InsufficientColor = "#F56C6C"
	}

	result, err := app.DB.Exec("UPDATE markers SET latitude = ?, longitude = ?, value = ?, required_value = ?, description = ?, sufficient_color = ?, insufficient_color = ? WHERE id = ?",
		marker.Latitude, marker.Longitude, marker.Value, marker.RequiredValue, marker.Description, marker.SufficientColor, marker.InsufficientColor, id)
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

	// 记录更新标记点的操作
	app.recordUserAction(c, "update_marker",
		fmt.Sprintf("更新标记点 #%s (%.6f, %.6f)", id, marker.Latitude, marker.Longitude),
		id)

	app.Logger.Info("更新标记点",
		zap.String("id", id),
		zap.Float64("latitude", marker.Latitude),
		zap.Float64("longitude", marker.Longitude))

	c.JSON(http.StatusOK, marker)
}

func (app *App) DeleteMarker(c *gin.Context) {
	id := c.Param("id")

	// 获取标记点信息用于记录
	var lat, lng float64
	err := app.DB.QueryRow("SELECT latitude, longitude FROM markers WHERE id = ?", id).Scan(&lat, &lng)
	if err != nil && err != sql.ErrNoRows {
		app.Logger.Error("查询标记点失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

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

	// 记录删除标记点的操作
	app.recordUserAction(c, "delete_marker",
		fmt.Sprintf("删除标记点 #%s (%.6f, %.6f)", id, lat, lng),
		id)

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

		// 记录上传图片的操作
		app.recordUserAction(c, "upload_image",
			fmt.Sprintf("为标记点 #%s 上传图片 %s", markerID, filename),
			markerID)

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

	// 记录删除图片的操作
	app.recordUserAction(c, "delete_image",
		fmt.Sprintf("从标记点 #%s 删除图片 %s", markerID, filename),
		markerID)

	app.Logger.Info("删除图片成功",
		zap.String("marker_id", markerID),
		zap.String("filename", filename))

	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

func (app *App) GetMarkers(c *gin.Context) {
	rows, err := app.DB.Query("SELECT id, latitude, longitude, value, required_value, description, sufficient_color, insufficient_color FROM markers")
	if err != nil {
		app.Logger.Error("查询标记点失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	markers := make(map[int]*Marker)
	for rows.Next() {
		var m Marker
		if err := rows.Scan(&m.ID, &m.Latitude, &m.Longitude, &m.Value, &m.RequiredValue, &m.Description, &m.SufficientColor, &m.InsufficientColor); err != nil {
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

func (app *App) recordVisit(c *gin.Context) {
	ip := c.ClientIP()
	userAgent := c.Request.UserAgent()
	path := c.Request.URL.Path
	referer := c.Request.Referer()

	_, err := app.DB.Exec(`
		INSERT INTO visits (ip, user_agent, path, referer)
		VALUES (?, ?, ?, ?)
	`, ip, userAgent, path, referer)

	if err != nil {
		app.Logger.Error("记录访问失败",
			zap.Error(err),
			zap.String("ip", ip),
			zap.String("path", path))
	}
}

func (app *App) recordUserAction(c *gin.Context, actionType string, actionDetail string, targetID string) {
	ip := c.ClientIP()
	userAgent := c.Request.UserAgent()

	_, err := app.DB.Exec(`
		INSERT INTO user_actions (ip, user_agent, action_type, action_detail, target_id)
		VALUES (?, ?, ?, ?, ?)
	`, ip, userAgent, actionType, actionDetail, targetID)

	if err != nil {
		app.Logger.Error("记录用户操作失败",
			zap.Error(err),
			zap.String("ip", ip),
			zap.String("action_type", actionType))
	}
}

func (app *App) GetVisits(c *gin.Context) {
	rows, err := app.DB.Query(`
		SELECT 
			v.ip,
			v.user_agent,
			COUNT(DISTINCT v.id) as visit_count,
			MAX(v.visit_time) as last_visit,
			GROUP_CONCAT(DISTINCT v.path) as paths,
			GROUP_CONCAT(DISTINCT v.referer) as referers,
			GROUP_CONCAT(
				json_object(
					'type', ua.action_type,
					'detail', ua.action_detail,
					'target', ua.target_id,
					'time', ua.action_time
				)
			) as actions
		FROM visits v
		LEFT JOIN user_actions ua ON v.ip = ua.ip AND v.user_agent = ua.user_agent
		GROUP BY v.ip, v.user_agent
		ORDER BY last_visit DESC
		LIMIT 1000
	`)
	if err != nil {
		app.Logger.Error("查询访问记录失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var visits []map[string]interface{}
	for rows.Next() {
		var ip, userAgent, paths, referers, actionsJson string
		var visitCount int
		var lastVisit string
		if err := rows.Scan(&ip, &userAgent, &visitCount, &lastVisit, &paths, &referers, &actionsJson); err != nil {
			app.Logger.Error("扫描访问记录失败", zap.Error(err))
			continue
		}

		uniquePaths := make(map[string]bool)
		for _, path := range strings.Split(paths, ",") {
			if path != "" {
				uniquePaths[path] = true
			}
		}

		uniqueReferers := make(map[string]bool)
		for _, referer := range strings.Split(referers, ",") {
			if referer != "" {
				uniqueReferers[referer] = true
			}
		}

		pathsList := make([]string, 0, len(uniquePaths))
		for path := range uniquePaths {
			pathsList = append(pathsList, path)
		}

		referersList := make([]string, 0, len(uniqueReferers))
		for referer := range uniqueReferers {
			referersList = append(referersList, referer)
		}

		// 解析操作记录
		var actions []map[string]interface{}
		if actionsJson != "" && actionsJson != "null" {
			actionStrings := strings.Split(actionsJson, "},{")
			for _, actionStr := range actionStrings {
				actionMap := make(map[string]interface{})
				// 简单解析 JSON 对象字符串
				actionStr = strings.Trim(actionStr, "[]}{")
				pairs := strings.Split(actionStr, ",")
				for _, pair := range pairs {
					kv := strings.Split(pair, ":")
					if len(kv) == 2 {
						key := strings.Trim(kv[0], "\" ")
						value := strings.Trim(kv[1], "\" ")
						actionMap[key] = value
					}
				}
				if len(actionMap) > 0 {
					actions = append(actions, actionMap)
				}
			}
		}

		visits = append(visits, map[string]interface{}{
			"ip":         ip,
			"userAgent":  userAgent,
			"visitCount": visitCount,
			"lastVisit":  lastVisit,
			"paths":      pathsList,
			"referers":   referersList,
			"actions":    actions,
		})
	}

	c.JSON(http.StatusOK, visits)
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

	app := &App{DB: db, Cfg: cfg, Logger: logger.Log}

	// 自定义静态文件处理
	r.Use(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/uploads/") {
			c.Header("Cache-Control", "public, max-age=3600")
		}
		app.recordVisit(c)
		c.Next()
	})
	r.Static("/uploads", cfg.Server.UploadDir)
	r.Static("/static", "./static")

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
		api.GET("/visits", app.GetVisits)

		// 高德地图静态图API代理
		api.GET("/amap-staticmap", func(c *gin.Context) {
			// 构建高德地图静态图API的URL
			amapURL := "https://restapi.amap.com/v3/staticmap?key=" + cfg.Map.APIKey

			// 将请求参数传递给高德API
			queryParams := c.Request.URL.Query()
			for key, values := range queryParams {
				for _, value := range values {
					if key != "key" { // 不传递客户端的key，使用服务器配置的key
						amapURL += "&" + key + "=" + value
					}
				}
			}

			// 使用http客户端请求高德地图API
			resp, err := http.Get(amapURL)
			if err != nil {
				logger.Log.Error("请求高德地图API失败", zap.Error(err), zap.String("url", amapURL))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "请求高德地图API失败"})
				return
			}
			defer resp.Body.Close()

			// 设置响应头
			for key, values := range resp.Header {
				for _, value := range values {
					c.Header(key, value)
				}
			}

			// 设置CORS头，允许任何来源访问
			c.Header("Access-Control-Allow-Origin", "*")

			// 设置状态码
			c.Status(resp.StatusCode)

			// 将高德地图API的响应直接传递给客户端
			c.DataFromReader(resp.StatusCode, resp.ContentLength, resp.Header.Get("Content-Type"), resp.Body, nil)
		})
	}

	r.GET("/admin", func(c *gin.Context) {
		c.File("./static/index.html")
	})
	r.GET("/view", func(c *gin.Context) {
		c.File("./static/view.html")
	})
	r.GET("/pdf-report", func(c *gin.Context) {
		c.File("./static/pdf-report.html")
	})
	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/admin")
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
