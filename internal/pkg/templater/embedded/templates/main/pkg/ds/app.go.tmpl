package ds

import (
	"context"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/errgroup"
)

type IService interface {
	InitService(ctx context.Context, drvs []Runnable, bucket ServerBucket, m *prometheus.Registry) error
	HitInfo(ctx context.Context, method string, u *url.URL, status int, contentLength int, ip string, contentType string, userAgent string, referer string, execTime float64)
	BeforeRunHook(ctx context.Context) error
	GetBucket() ServerBucket
	GetAuthorizer() Authorizer
}

type Actor interface {
	GetID() int64
}

type Authorizer interface {
	Init(ctx context.Context, drvs []Runnable, m *prometheus.Registry) (Authorizer, error)
	AuthRest(r *http.Request) (Actor, error)
	CheckCSRF(r *http.Request) (bool, error)
}

type Namable interface {
	Name() string
}

type OnlyRunnable interface {
	Run(ctx context.Context, errGr *errgroup.Group)
	Shutdown(ctx context.Context) error
	GracefulStop(ctx context.Context) (<-chan struct{}, error)
}

// Todo возможно надо передавать appName в Init
type Runnable interface {
	Init(ctx context.Context, serviceName string, rb ServerBucket, metrics *prometheus.Registry) error
	Namable
	OnlyRunnable
}

type RunnableService interface {
	Namable
	Init(ctx context.Context, serviceName, appName string, metrics *prometheus.Registry, srv IService) error
	Initialization(ctx context.Context) error
	OnlyRunnable
}

type AppInfo struct {
	AppName     string `json:"app_name"`
	Version     string `json:"version"`
	BuildTime   string `json:"build_time"`
	BuildOS     string `json:"build_os"`
	BuildCommit string `json:"build_commit"`
	StartupTime string `json:"startup_time"`
}

type AuthorizationData struct {
	UserID int64
}

// ToDo Аккумулировать все ready флаги в структуру bucket
type ServerBucket struct {
	AppInfo  *AppInfo
	AppReady *atomic.Bool
}

func NewAppInfo(name string) *AppInfo {
	return &AppInfo{
		AppName:     name,
		StartupTime: time.Now().Format(time.RFC3339),
	}
}

func (i *AppInfo) WithVersion(version string) *AppInfo {
	i.Version = version
	return i
}

func (i *AppInfo) WithBuildTime(buildTime string) *AppInfo {
	i.BuildTime = buildTime
	return i
}

func (i *AppInfo) WithBuildOS(buildOS string) *AppInfo {
	i.BuildOS = buildOS
	return i
}

func (i *AppInfo) WithBuildCommit(commit string) *AppInfo {
	i.BuildCommit = commit
	return i
}
