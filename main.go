package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"github.com/disintegration/imaging"
	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"gopkg.in/yaml.v2"
)

const (
	uploadDir     = "./image"
	thumbnailDir  = "./thumb"
	maxUploadSize = 40 * 1024 * 1024 // 40MB
)

type Config struct {
	Database struct {
		User     string `yaml:"user"`
		Password string `yaml:"password"`
		DBName   string `yaml:"dbname"`
		Host     string `yaml:"host"`
		Port     int    `yaml:"port"`
		SSLMode  string `yaml:"sslmode"`
	} `yaml:"database"`
}

type ImageInfo struct {
	ID                int    `json:"id"`
	Filename          string `json:"filename"`
	ThumbnailFilename string `json:"thumbnail_filename"`
	Width             int    `json:"width"`
	Height            int    `json:"height"`
	SHA256Sum         string `json:"sha256sum"`
	UploadDate        string `json:"upload_date"`
	ThumbnailPath     string `json:"thumbnail_path"`
	ImagePath         string `json:"image_path"`
}

var (
	db  *sql.DB
	cfg Config
)

func loadConfig() error {
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		return fmt.Errorf("error reading config file: %v", err)
	}

	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		return fmt.Errorf("error parsing config file: %v", err)
	}

	return nil
}

func main() {
	if err := loadConfig(); err != nil {
		log.Fatal(err)
	}

	dbConnStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.DBName,
		cfg.Database.SSLMode)

	var err error
	db, err = sql.Open("postgres", dbConnStr)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Test database connection
	err = db.Ping()
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	router := gin.Default()
	router.LoadHTMLGlob("templates/*")
	router.Static("/image", "./image")
	router.Static("/thumb", "./thumb")
	router.POST("/upload", uploadHandler)
	router.GET("/view", viewHandler)
	router.Run(":8080")
}

func uploadHandler(c *gin.Context) {
	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	files := form.File["file"]

	responses := make([]gin.H, 0)

	for _, file := range files {
		response, statusCode := processFile(file)
		response["filename"] = file.Filename
		responses = append(responses, response)

		if statusCode != http.StatusOK {
			c.JSON(statusCode, responses)
			return
		}
	}

	c.JSON(http.StatusOK, responses)
}

func processFile(file *multipart.FileHeader) (gin.H, int) {
	if file.Size > maxUploadSize {
		return gin.H{"error": "File too large"}, http.StatusBadRequest
	}

	src, err := file.Open()
	if err != nil {
		return gin.H{"error": err.Error()}, http.StatusInternalServerError
	}
	defer src.Close()

	buff := make([]byte, 512)
	_, err = src.Read(buff)
	if err != nil {
		return gin.H{"error": "Failed to read file"}, http.StatusInternalServerError
	}
	filetype := http.DetectContentType(buff)
	if filetype != "image/jpeg" && filetype != "image/png" {
		return gin.H{"error": "File type not allowed. Only JPG and PNG are allowed."}, http.StatusBadRequest
	}

	src.Seek(0, 0)

	hash := sha256.New()
	if _, err := io.Copy(hash, src); err != nil {
		return gin.H{"error": "Failed to generate SHA-256"}, http.StatusInternalServerError
	}
	sha256sum := hex.EncodeToString(hash.Sum(nil))

	var exists bool
	err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM images WHERE sha256sum = $1)", sha256sum).Scan(&exists)
	if err != nil {
		return gin.H{"error": "Database error"}, http.StatusInternalServerError
	}
	if exists {
		return gin.H{"error": "File already exists"}, http.StatusConflict
	}

	src.Seek(0, 0)

	filename := sha256sum + filepath.Ext(file.Filename)
	if err := saveFile(src, filename); err != nil {
		return gin.H{"error": "Failed to save file"}, http.StatusInternalServerError
	}

	thumbnailFilename, err := generateThumbnail(filename)
	if err != nil {
		return gin.H{"error": "Failed to generate thumbnail"}, http.StatusInternalServerError
	}

	if err := saveToDatabase(filename, thumbnailFilename, sha256sum); err != nil {
		return gin.H{"error": "Failed to save to database"}, http.StatusInternalServerError
	}

	return gin.H{"message": "File uploaded successfully", "sha256sum": sha256sum}, http.StatusOK
}

func saveFile(file multipart.File, filename string) error {
	dst, err := os.Create(filepath.Join(uploadDir, filename))
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, file)
	return err
}

func generateThumbnail(filename string) (string, error) {
	src, err := imaging.Open(filepath.Join(uploadDir, filename))
	if err != nil {
		return "", err
	}

	thumbnail := imaging.Thumbnail(src, 120, 120, imaging.CatmullRom)

	thumbnailFilename := filepath.Base(filename)
	thumbnailFilename = thumbnailFilename[:len(thumbnailFilename)-len(filepath.Ext(thumbnailFilename))] + ".jpg"
	err = imaging.Save(thumbnail, filepath.Join(thumbnailDir, thumbnailFilename))
	if err != nil {
		return "", err
	}

	return thumbnailFilename, nil
}

func saveToDatabase(filename, thumbnailFilename, sha256sum string) error {
	img, err := os.Open(filepath.Join(uploadDir, filename))
	if err != nil {
		return err
	}
	defer img.Close()

	config, _, err := image.DecodeConfig(img)
	if err != nil {
		return err
	}

	_, err = db.Exec("INSERT INTO images (filename, thumbnail_filename, width, height, sha256sum) VALUES ($1, $2, $3, $4, $5)",
		filename, thumbnailFilename, config.Width, config.Height, sha256sum)
	return err
}

func viewHandler(c *gin.Context) {
	images, err := getRecentImages(100)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"error": "Failed to fetch images"})
		return
	}

	c.HTML(http.StatusOK, "view.html", gin.H{
		"images": images,
	})
}

func getRecentImages(limit int) ([]ImageInfo, error) {
	query := `
		SELECT id, filename, thumbnail_filename, width, height, sha256sum, upload_date
		FROM images
		ORDER BY upload_date DESC
		LIMIT $1
	`

	rows, err := db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var images []ImageInfo
	for rows.Next() {
		var img ImageInfo
		err := rows.Scan(
			&img.ID,
			&img.Filename,
			&img.ThumbnailFilename,
			&img.Width,
			&img.Height,
			&img.SHA256Sum,
			&img.UploadDate,
		)
		if err != nil {
			return nil, err
		}
		img.ThumbnailPath = filepath.Join("/thumb", img.ThumbnailFilename)
		img.ImagePath = filepath.Join("/image", img.Filename)
		images = append(images, img)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return images, nil
}
