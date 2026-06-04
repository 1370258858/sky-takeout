package common

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// Config describes infrastructure connection settings.
type Config struct {
	MySQLDSN      string
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	MQURL         string
}

// Resources exposes initialized clients for business layer usage.
type Resources struct {
	db     *gorm.DB
	redis  *redis.Client
	mqConn *amqp.Connection
	mqCh   *amqp.Channel
	cfg    Config
}

// NewResources initializes mysql, redis and mq clients.
func NewResources(cfg Config) (*Resources, error) {
	res := &Resources{cfg: cfg}

	if err := res.initMySQL(); err != nil {
		return nil, err
	}
	if err := res.initRedis(); err != nil {
		return nil, err
	}
	if err := res.initMQ(); err != nil {
		return nil, err
	}

	return res, nil
}

// MustNewResources creates resources or panics on any error.
func MustNewResources(cfg Config) *Resources {
	res, err := NewResources(cfg)
	if err != nil {
		panic(err)
	}
	return res
}

// LoadConfigFromEnv loads config from env with service-specific prefix fallback.
// Example prefix: "ADMIN_SERVICE".
func LoadConfigFromEnv(prefix string) Config {
	prefix = normalizePrefix(prefix)

	cfg := Config{
		MySQLDSN:      pickEnv(prefix+"_MYSQL_DSN", "MYSQL_DSN"),
		RedisAddr:     pickEnv(prefix+"_REDIS_ADDR", "REDIS_ADDR"),
		RedisPassword: pickEnv(prefix+"_REDIS_PASSWORD", "REDIS_PASSWORD"),
		MQURL:         pickEnv(prefix+"_MQ_URL", "MQ_URL"),
	}

	if cfg.MySQLDSN == "" {
		// Local debug default: connect to docker-mapped mysql port on host.
		cfg.MySQLDSN = "sky:sky@tcp(127.0.0.1:3306)/sky_takeout?charset=utf8mb4&parseTime=True&loc=Local"
	}

	if cfg.RedisAddr == "" {
		cfg.RedisAddr = "127.0.0.1:6379"
	}
	if cfg.MQURL == "" {
		cfg.MQURL = "amqp://guest:guest@127.0.0.1:5672/"
	}

	redisDBStr := pickEnv(prefix+"_REDIS_DB", "REDIS_DB")
	if redisDBStr != "" {
		if n, err := strconv.Atoi(redisDBStr); err == nil {
			cfg.RedisDB = n
		}
	}

	return cfg
}

// DB returns initialized mysql client.
func (r *Resources) DB() *gorm.DB { return r.db }

// Redis returns initialized redis client.
func (r *Resources) Redis() *redis.Client { return r.redis }

// MQ returns initialized mq channel and connection.
func (r *Resources) MQ() (*amqp.Connection, *amqp.Channel) { return r.mqConn, r.mqCh }

// PublishJSON publishes message body to exchange/routingKey.
func (r *Resources) PublishJSON(exchange, routingKey string, body []byte) error {
	if r.mqCh == nil {
		return errors.New("mq channel not initialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return r.mqCh.PublishWithContext(ctx, exchange, routingKey, false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        body,
	})
}

// Close gracefully closes mq and redis resources.
func (r *Resources) Close() error {
	var errs []string

	if r.mqCh != nil {
		if err := r.mqCh.Close(); err != nil {
			errs = append(errs, "close mq channel: "+err.Error())
		}
	}
	if r.mqConn != nil {
		if err := r.mqConn.Close(); err != nil {
			errs = append(errs, "close mq connection: "+err.Error())
		}
	}
	if r.redis != nil {
		if err := r.redis.Close(); err != nil {
			errs = append(errs, "close redis: "+err.Error())
		}
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func (r *Resources) initMySQL() error {
	if strings.TrimSpace(r.cfg.MySQLDSN) == "" {
		return errors.New("mysql dsn is empty")
	}
	gormDB, err := gorm.Open(mysql.Open(r.cfg.MySQLDSN), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("init mysql: %w", err)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		return fmt.Errorf("get sql db: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("ping mysql: %w", err)
	}
	r.db = gormDB
	return nil
}

func (r *Resources) initRedis() error {
	client := redis.NewClient(&redis.Options{
		Addr:     r.cfg.RedisAddr,
		Password: r.cfg.RedisPassword,
		DB:       r.cfg.RedisDB,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("ping redis: %w", err)
	}
	r.redis = client
	return nil
}

func (r *Resources) initMQ() error {
	conn, err := amqp.Dial(r.cfg.MQURL)
	if err != nil {
		return fmt.Errorf("dial mq: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("open mq channel: %w", err)
	}
	r.mqConn = conn
	r.mqCh = ch
	return nil
}

func normalizePrefix(prefix string) string {
	prefix = strings.TrimSpace(strings.ToUpper(prefix))
	prefix = strings.ReplaceAll(prefix, "-", "_")
	prefix = strings.ReplaceAll(prefix, " ", "_")
	return prefix
}

func pickEnv(primary, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(primary)); v != "" {
		return v
	}
	return strings.TrimSpace(os.Getenv(fallback))
}
