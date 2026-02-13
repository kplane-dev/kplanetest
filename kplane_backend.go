package kplanetest

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/kcp-dev/embeddedetcd"
	embeddedoptions "github.com/kcp-dev/embeddedetcd/options"
	"k8s.io/apiserver/pkg/server/options"
	"k8s.io/apiserver/pkg/storage/storagebackend"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

const (
	defaultRootControlPlane = "root"
	kplaneReadyTimeout      = 60 * time.Second
	kplaneModuleVersion     = "v0.0.6"
	kplaneAssetsEnv         = "KPLANETEST_ASSETS"
	kplaneBinaryEnv         = "KPLANETEST_APISERVER_BINARY"
)

// KplaneBackend starts a shared kplane-dev/apiserver and points envtest to it.
// It uses kcp embeddedetcd for local in-process etcd.
type KplaneBackend struct {
	runtime *kplaneRuntime
}

func NewKplaneBackend() *KplaneBackend {
	return &KplaneBackend{runtime: &kplaneRuntime{}}
}

func (b *KplaneBackend) Start(env *envtest.Environment) (*rest.Config, error) {
	cfg, err := b.runtime.acquire(env)
	if err != nil {
		return nil, err
	}

	useExisting := true
	env.UseExistingCluster = &useExisting
	env.Config = rest.CopyConfig(cfg)
	env.KubeConfig = nil

	startedCfg, err := env.Start()
	if err != nil {
		_ = b.runtime.release(env)
		return nil, err
	}
	return startedCfg, nil
}

func (b *KplaneBackend) Stop(env *envtest.Environment) error {
	stopErr := env.Stop()
	releaseErr := b.runtime.release(env)
	if stopErr != nil {
		return stopErr
	}
	return releaseErr
}

type kplaneRuntime struct {
	mu        sync.Mutex
	totalRefs int
	nextID    int
	envRefs   map[*envtest.Environment]int
	envIDs    map[*envtest.Environment]string

	cancel context.CancelFunc
	cmd    *exec.Cmd
	tmpDir string

	baseURL string
	token   string

	logMu sync.Mutex
	logs  bytes.Buffer
}

func (r *kplaneRuntime) acquire(env *envtest.Environment) (*rest.Config, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if env == nil {
		return nil, fmt.Errorf("nil environment")
	}
	if id, ok := r.envIDs[env]; ok {
		r.envRefs[env]++
		r.totalRefs++
		return r.configForCluster(id), nil
	}

	if r.totalRefs == 0 {
		if err := r.startLocked(); err != nil {
			return nil, err
		}
	}
	if r.envRefs == nil {
		r.envRefs = make(map[*envtest.Environment]int)
	}
	if r.envIDs == nil {
		r.envIDs = make(map[*envtest.Environment]string)
	}
	r.nextID++
	clusterID := fmt.Sprintf("kpt-%d", r.nextID)
	r.envIDs[env] = clusterID
	r.envRefs[env] = 1
	r.totalRefs++
	return r.configForCluster(clusterID), nil
}

func (r *kplaneRuntime) release(env *envtest.Environment) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if env == nil || r.totalRefs == 0 {
		return nil
	}
	count := r.envRefs[env]
	if count == 0 {
		return nil
	}
	if count == 1 {
		delete(r.envRefs, env)
		delete(r.envIDs, env)
	} else {
		r.envRefs[env] = count - 1
	}
	r.totalRefs--
	if r.totalRefs > 0 {
		return nil
	}
	return r.stopLocked()
}

func (r *kplaneRuntime) configForCluster(clusterID string) *rest.Config {
	return &rest.Config{
		Host:        fmt.Sprintf("%s/clusters/%s/control-plane", r.baseURL, clusterID),
		BearerToken: r.token,
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: true, // test harness only
		},
		QPS:   1000.0,
		Burst: 2000.0,
	}
}

func (r *kplaneRuntime) startLocked() error {
	return r.startLockedWithEtcdTLS(defaultEmbeddedEtcdTLS())
}

func defaultEmbeddedEtcdTLS() bool {
	// Default off for reliability across host/toolchain combinations.
	// Set KPLANETEST_EMBEDDED_ETCD_TLS=1 to force embedded-etcd TLS mode.
	return os.Getenv("KPLANETEST_EMBEDDED_ETCD_TLS") == "1"
}

func (r *kplaneRuntime) startLockedWithEtcdTLS(useEtcdTLS bool) error {
	tmpDir, err := os.MkdirTemp("", "kplanetest-kplane-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	r.tmpDir = tmpDir

	clientPort, err := reserveFreePort()
	if err != nil {
		return err
	}
	peerPort, err := reserveFreePort()
	if err != nil {
		return err
	}
	securePort, err := reserveFreePort()
	if err != nil {
		return err
	}

	etcdOpts := embeddedoptions.NewOptions(tmpDir)
	etcdOpts.Enabled = useEtcdTLS
	etcdOpts.ClientPort = strconv.Itoa(clientPort)
	etcdOpts.PeerPort = strconv.Itoa(peerPort)

	storageCfg := storagebackend.NewDefaultConfig("/registry", nil)
	kubeEtcdOpts := options.NewEtcdOptions(storageCfg)
	completedEtcdOpts := etcdOpts.Complete(kubeEtcdOpts)
	etcdConfig, err := embeddedetcd.NewConfig(completedEtcdOpts, true)
	if err != nil {
		return fmt.Errorf("build embedded etcd config: %w", err)
	}
	if !useEtcdTLS {
		etcdConfig.Config.ListenPeerUrls = []url.URL{{Scheme: "http", Host: "localhost:" + etcdOpts.PeerPort}}
		etcdConfig.Config.AdvertisePeerUrls = []url.URL{{Scheme: "http", Host: "localhost:" + etcdOpts.PeerPort}}
		etcdConfig.Config.ListenClientUrls = []url.URL{{Scheme: "http", Host: "localhost:" + etcdOpts.ClientPort}}
		etcdConfig.Config.AdvertiseClientUrls = []url.URL{{Scheme: "http", Host: "localhost:" + etcdOpts.ClientPort}}
		etcdConfig.Config.InitialCluster = etcdConfig.Config.InitialClusterFromName(etcdConfig.Config.Name)
	}

	ctx, cancel := context.WithCancel(context.Background())
	if err := embeddedetcd.NewServer(etcdConfig.Complete()).Run(ctx); err != nil {
		cancel()
		return fmt.Errorf("start embedded etcd: %w", err)
	}

	token, tokenFile, err := writeTokenAuthFile(tmpDir)
	if err != nil {
		cancel()
		return err
	}
	saKeyFile, err := writeRSAKey(filepath.Join(tmpDir, "sa.key"))
	if err != nil {
		cancel()
		return err
	}

	bin, err := ensureKplaneBinary()
	if err != nil {
		cancel()
		return err
	}

	rootCluster := defaultRootControlPlane
	etcdScheme := "https"
	if !useEtcdTLS {
		etcdScheme = "http"
	}
	etcdEndpoint := fmt.Sprintf("%s://localhost:%d", etcdScheme, clientPort)
	args := []string{
		"--etcd-servers=" + etcdEndpoint,
		"--cert-dir=" + filepath.Join(tmpDir, "certs"),
		"--secure-port=" + strconv.Itoa(securePort),
		"--enable-aggregator-routing=true",
		"--authorization-mode=RBAC",
		"--anonymous-auth=true",
		"--token-auth-file=" + tokenFile,
		"--allow-privileged=true",
		"--service-cluster-ip-range=10.0.0.0/24",
		"--service-account-issuer=kplanetest",
		"--service-account-signing-key-file=" + saKeyFile,
		"--service-account-key-file=" + saKeyFile,
		"--root-control-plane-name=" + rootCluster,
		"--v=2",
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("start kplane apiserver: %w", err)
	}

	r.cancel = cancel
	r.cmd = cmd
	go r.captureLogs("stdout", stdout)
	go r.captureLogs("stderr", stderr)

	baseURL := fmt.Sprintf("https://127.0.0.1:%d", securePort)
	if err := r.waitReady(baseURL, rootCluster, token); err != nil {
		_ = r.stopLocked()
		return err
	}

	r.baseURL = baseURL
	r.token = token
	r.envRefs = make(map[*envtest.Environment]int)
	r.envIDs = make(map[*envtest.Environment]string)
	return nil
}

func (r *kplaneRuntime) stopLocked() error {
	if r.cancel != nil {
		r.cancel()
	}

	if r.cmd != nil && r.cmd.Process != nil {
		_ = r.cmd.Process.Signal(syscall.SIGTERM)
		done := make(chan error, 1)
		go func() { done <- r.cmd.Wait() }()
		select {
		case <-time.After(5 * time.Second):
			_ = r.cmd.Process.Kill()
			<-done
		case <-done:
		}
	}

	if r.tmpDir != "" {
		_ = os.RemoveAll(r.tmpDir)
	}

	r.cancel = nil
	r.cmd = nil
	r.tmpDir = ""
	r.baseURL = ""
	r.token = ""
	r.totalRefs = 0
	r.nextID = 0
	r.envRefs = nil
	r.envIDs = nil
	r.logs.Reset()
	return nil
}

func (r *kplaneRuntime) captureLogs(stream string, src io.ReadCloser) {
	if src == nil {
		return
	}
	defer src.Close()
	scanner := bufio.NewScanner(src)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 2*1024*1024)

	r.logMu.Lock()
	r.logs.WriteString("\n--- ")
	r.logs.WriteString(stream)
	r.logs.WriteString(" ---\n")
	r.logMu.Unlock()

	for scanner.Scan() {
		r.logMu.Lock()
		r.logs.WriteString(scanner.Text())
		r.logs.WriteString("\n")
		r.logMu.Unlock()
	}
}

func (r *kplaneRuntime) waitReady(baseURL, rootCluster, token string) error {
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tlsConfigInsecure,
		},
	}
	url := fmt.Sprintf("%s/clusters/%s/control-plane/readyz", baseURL, rootCluster)
	deadline := time.Now().Add(kplaneReadyTimeout)

	var lastErr error
	for time.Now().Before(deadline) {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := client.Do(req)
		if err == nil && resp != nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			err = fmt.Errorf("readyz status=%d", resp.StatusCode)
		}
		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("kplane apiserver not ready: %v\nlogs:\n%s", lastErr, r.logString())
}

func (r *kplaneRuntime) logString() string {
	r.logMu.Lock()
	defer r.logMu.Unlock()
	return r.logs.String()
}

var tlsConfigInsecure = tls.Config{InsecureSkipVerify: true}

var (
	kplaneBinaryOnce sync.Once
	kplaneBinaryPath string
	kplaneBinaryErr  error
)

func ensureKplaneBinary() (string, error) {
	kplaneBinaryOnce.Do(func() {
		if configured, err := resolveConfiguredKplaneBinary(); err != nil {
			kplaneBinaryErr = err
			return
		} else if configured != "" {
			kplaneBinaryPath = configured
			return
		}

		out := filepath.Join(os.TempDir(), "kplanetest-kplane-apiserver")

		downloadCmd := exec.Command("go", "mod", "download", "-json", "github.com/kplane-dev/apiserver@"+kplaneModuleVersion)
		rawDownload, err := downloadCmd.CombinedOutput()
		if err != nil {
			kplaneBinaryErr = fmt.Errorf("resolve kplane module dir: %w (%s)", err, strings.TrimSpace(string(rawDownload)))
			return
		}
		var moduleInfo struct {
			Dir string
		}
		if err := json.Unmarshal(rawDownload, &moduleInfo); err != nil {
			kplaneBinaryErr = fmt.Errorf("parse kplane module metadata: %w", err)
			return
		}
		moduleDir := strings.TrimSpace(moduleInfo.Dir)
		if moduleDir == "" {
			kplaneBinaryErr = fmt.Errorf("kplane module metadata did not include module dir")
			return
		}

		buildCmd := exec.Command("go", "build", "-o", out, "./cmd/apiserver")
		buildCmd.Dir = moduleDir
		rawBuild, err := buildCmd.CombinedOutput()
		if err != nil {
			kplaneBinaryErr = fmt.Errorf("build kplane apiserver: %w (%s)", err, strings.TrimSpace(string(rawBuild)))
			return
		}
		kplaneBinaryPath = out
	})
	return kplaneBinaryPath, kplaneBinaryErr
}

func resolveConfiguredKplaneBinary() (string, error) {
	if explicit := strings.TrimSpace(os.Getenv(kplaneBinaryEnv)); explicit != "" {
		if err := validateBinaryPath(explicit); err != nil {
			return "", fmt.Errorf("%s points to invalid binary %q: %w", kplaneBinaryEnv, explicit, err)
		}
		return explicit, nil
	}

	if assetsDir := strings.TrimSpace(os.Getenv(kplaneAssetsEnv)); assetsDir != "" {
		path := filepath.Join(assetsDir, "kplane-apiserver")
		if err := validateBinaryPath(path); err != nil {
			return "", fmt.Errorf("%s=%q did not contain usable kplane-apiserver: %w", kplaneAssetsEnv, assetsDir, err)
		}
		return path, nil
	}

	return "", nil
}

func validateBinaryPath(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("path is a directory")
	}
	return nil
}

func reserveFreePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("reserve free port: %w", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

func writeTokenAuthFile(dir string) (token, path string, err error) {
	token = "kplanetest-token"
	path = filepath.Join(dir, "tokens.csv")
	line := fmt.Sprintf("%s,admin,admin,system:masters\n", token)
	if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
		return "", "", fmt.Errorf("write token auth file: %w", err)
	}
	return token, path, nil
}

func writeRSAKey(path string) (string, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", fmt.Errorf("generate rsa key: %w", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
		return "", fmt.Errorf("write rsa key: %w", err)
	}
	return path, nil
}
