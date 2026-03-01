package storage

import (
	"chatbot/pkg/config"
	"database/sql"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	_ "github.com/go-sql-driver/mysql" // Import MySQL driver
	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/zerolog/log"
	gormzerolog "github.com/vitaliy-art/gorm-zerolog"
	"gorm.io/driver/mysql" // Import GORM MySQL driver
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var (
	DB       *gorm.DB
	DBConfig *config.StorageConfig
)

const (
	DefaultDriveName       = "sqlite3"
	DefaultConnMaxIdleTime = 15 * time.Minute
	DefaultConnMaxLifetime = 1 * time.Hour
	DefaultMaxIdleConns    = 10
	DefaultMaxOpenConns    = 100
)

// maxRetries 是尝试重连的最大次数
const maxRetries = 5

// retryDelayMin 和 retryDelayMax 分别是重试延迟的最小和最大时间范围
const (
	retryDelayMin = 5 * time.Second
	retryDelayMax = 30 * time.Second
)

func InitWithConfig(cfg config.SqlDBConfig) *gorm.DB {
	var gormDB *gorm.DB
	var err error
	gormDB, err = ConnectDB(DBConfig.Provider, &cfg)
	if err != nil {
		panic(fmt.Sprintf("connect database failed:%v", err))
	}
	log.Info().Msg("connect database success")
	return gormDB
}

// Configure sets the storage configuration before the first InitDB call.
// It must be called once from app startup before any repo initialisation.
func Configure(cfg *config.StorageConfig) {
	DBConfig = cfg
}

func InitDB() *gorm.DB {
	if db := DB; db != nil {
		return db
	}
	if DBConfig == nil {
		log.Error().Msg("storage not configured – call storage.Configure() before InitDB()")
		return nil
	}
	var gormDB *gorm.DB
	var err error
	switch DBConfig.Provider {
	case "sqlite":
		gormDB, err = ConnectDB(DefaultDriveName, DBConfig.SqlDB)
		if err != nil {
			panic(fmt.Sprintf("connect database failed:%v", err))
		}
		log.Info().Msg("connect database success")
	case "mysql":
		gormDB, err = ConnectDB("mysql", DBConfig.SqlDB)
		if err != nil {
			panic(fmt.Sprintf("connect database failed:%v", err))
		}
		log.Info().Msg("connect database success")
	default:
		// panic("not supported database type, please check your configuration")
	}
	return gormDB
}

func ConnectDB(drive string, cfg *config.SqlDBConfig) (*gorm.DB, error) {
	var db *sql.DB
	var err error
	var gormDB *gorm.DB

	switch drive {
	case "sqlite3":
		if cfg.Path == "" || cfg.Name == "" {
			log.Error().Msg("not configured database path or name yet")
			return nil, fmt.Errorf("not configured database path or name yet")
		}
		if info, err := os.Stat(cfg.Path); err != nil || !info.IsDir() {
			err := os.Mkdir(cfg.Path, 0766)
			if err != nil {
				panic(err)
			}
		}
		source := filepath.Join(cfg.Path, cfg.Name)
		err = CreateSqlFile(source)
		if err != nil {
			log.Error().Msg("no database file yet")
			return nil, fmt.Errorf("no database file yet: %s", source)
		}
		db, err = sql.Open("sqlite3", source)
		if err != nil {
			log.Error().Err(err).Msg("open sqlite database failed")
			if db != nil {
				db.Close()
			}
			return nil, err
		}
		gormDB, err = gorm.Open(sqlite.Dialector{Conn: db}, &gorm.Config{
			Logger: gormzerolog.NewGormLogger(),
		})
	case "mysql":
		if cfg.Host == "" || cfg.Port == "" || cfg.User == "" || cfg.DBName == "" {
			log.Error().Msg("not configured mysql database connection details yet")
			return nil, fmt.Errorf("not configured mysql database connection details yet")
		}
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=%s&parseTime=True&loc=Local",
			cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.DBName, cfg.Charset)
		if cfg.Charset == "" {
			dsn = fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
				cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.DBName)
		}
		db, err = sql.Open("mysql", dsn)
		if err != nil {
			log.Error().Err(err).Msg("open mysql database failed")
			if db != nil {
				db.Close()
			}
			return nil, err
		}
		gormDB, err = gorm.Open(mysql.New(mysql.Config{
			Conn: db,
		}), &gorm.Config{
			Logger: gormzerolog.NewGormLogger(),
		})
	default:
		return nil, fmt.Errorf("not supported database type: %s", drive)
	}

	if err != nil {
		log.Error().Err(err).Msg("gorm open database failed")
		if db != nil {
			db.Close()
		}
		return nil, err
	}

	// Set database connection parameters
	db.SetConnMaxIdleTime(DefaultConnMaxIdleTime)
	db.SetConnMaxLifetime(DefaultConnMaxLifetime)
	db.SetMaxIdleConns(DefaultMaxIdleConns)
	db.SetMaxOpenConns(DefaultMaxOpenConns)

	// Check if the database connection is alive
	if err = db.Ping(); err != nil {
		log.Error().Msg("database connection is not alive, check your database configuration")
		db.Close()
		return nil, err
	}

	DB = gormDB
	go isAlive()
	return DB, nil
}

// fixme 不好去测试
func isAlive() {
	for {
		timer := time.NewTimer(time.Minute * 10)
		defer timer.Stop() // 确保定时器被释放，避免资源泄漏

		select {
		case <-timer.C:
			if DB == nil {
				log.Error().Msg("database connection is not alive")
				return
			}
			// 尝试检查数据库连接
			err := checkDBConnection()
			if err != nil {
				log.Error().Err(err).Msg("database connection error")
				// 尝试重连
				if !reconnect(maxRetries) {
					log.Panic().Msg("failed to reconnect to the database after multiple attempts")
					return
				}
			}
		}
	}
}

// checkDBConnection 封装了检查数据库连接是否存活的逻辑
func checkDBConnection() error {
	if err := DB.Exec("SELECT 1").Error; err != nil {
		return err
	}
	return nil
}

// reconnect 尝试重连数据库，最多尝试maxRetries次
func reconnect(maxRetries int) bool {
	var retries int
	for retries < maxRetries {
		if err := Reconnect(DefaultDriveName, DBConfig.SqlDB); err == nil {
			// 重连成功，跳出循环
			return true
		}

		time.Sleep(randomRetryDelay())
		retries++
	}
	return false
}

// randomRetryDelay 返回一个随机的重试延迟时间，介于retryDelayMin和retryDelayMax之间
func randomRetryDelay() time.Duration {
	delay := time.Duration(rand.Intn(int(retryDelayMax-retryDelayMin))) + retryDelayMin
	return delay
}

func Reconnect(drive string, cfg *config.SqlDBConfig) error {
	var db *sql.DB
	var err error

	switch drive {
	case "sqlite3":
		if cfg.Path == "" || cfg.Name == "" {
			log.Error().Msg("not configured database path or name yet")
			return fmt.Errorf("not configured database path or name yet")
		}
		source := filepath.Join(cfg.Path, cfg.Name)
		err = CreateSqlFile(source)
		if err != nil {
			log.Error().Msg("no database file yet")
			return fmt.Errorf("no database file yet: %s", source)
		}
		db, err = sql.Open("sqlite3", source)
		if err != nil {
			log.Error().Err(err).Msg("open sqlite database failed")
			if db != nil {
				db.Close()
			}
			return err
		}
		DB, err = gorm.Open(sqlite.Dialector{Conn: db}, &gorm.Config{
			Logger: gormzerolog.NewGormLogger(),
		})
	case "mysql":
		if cfg.Host == "" || cfg.Port == "" || cfg.User == "" || cfg.DBName == "" {
			log.Error().Msg("not configured mysql database connection details yet")
			return fmt.Errorf("not configured mysql database connection details yet")
		}
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=%s&parseTime=True&loc=Local",
			cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.DBName, cfg.Charset)
		if cfg.Charset == "" {
			dsn = fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
				cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.DBName)
		}
		db, err = sql.Open("mysql", dsn)
		if err != nil {
			log.Error().Err(err).Msg("open mysql database failed")
			if db != nil {
				db.Close()
			}
			return err
		}
		DB, err = gorm.Open(mysql.New(mysql.Config{
			Conn: db,
		}), &gorm.Config{
			Logger: gormzerolog.NewGormLogger(),
		})
	default:
		return fmt.Errorf("not supported database type: %s", drive)
	}

	if err != nil {
		log.Error().Err(err).Msg("gorm open database failed")
		if db != nil {
			db.Close()
		}
		return err
	}

	// Set database connection parameters
	db.SetConnMaxIdleTime(DefaultConnMaxIdleTime)
	db.SetConnMaxLifetime(DefaultConnMaxLifetime)
	db.SetMaxIdleConns(DefaultMaxIdleConns)
	db.SetMaxOpenConns(DefaultMaxOpenConns)

	// Check if the database connection is alive
	if err = db.Ping(); err != nil {
		log.Error().Err(err).Msg("open database failed")
		db.Close()
		return err
	}
	return nil
}

func CreateSqlFile(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// 文件不存在
		// 创建文件
		file, err := os.Create(path)
		if err != nil {
			// 处理创建文件时的错误
			return err
		}
		defer file.Close()
		// 文件创建成功，可以在这里写入文件内容
		return nil
	} else {
		return nil
	}
}
