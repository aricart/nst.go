package nst

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type ExternalNatsServer struct {
	t       testing.TB
	Process *exec.Cmd
	Connections
}

type Ports struct {
	Nats       []string `json:"nats,omitempty"`
	Cluster    []string `json:"cluster,omitempty"`
	Monitoring []string `json:"monitoring,omitempty"`
	WebSocket  []string `json:"websocket,omitempty"`
}

func StartExternalProcessWithConfig(t testing.TB, fp string) *ExternalNatsServer {
	portsFileDir := t.TempDir()
	var conf *ResolverConf

	var process *exec.Cmd
	if fp == "" {
		file, err := os.CreateTemp(os.TempDir(), "nats-server-*.conf")
		require.NoError(t, err)
		_, err = file.Write(Conf{PortsFileDir: portsFileDir}.Marshal(t))
		require.NoError(t, err)
		require.NoError(t, file.Close())
		fp = file.Name()
	}

	conf = ParseConf(t, fp)
	wantsLog := conf.Debug || conf.Trace

	if conf.PortsFileDir == "" {
		conf.PortsFileDir = portsFileDir
		require.NoError(t, os.WriteFile(fp, conf.Marshal(t), 0o644))
	} else {
		portsFileDir = conf.PortsFileDir
	}
	var stdout, stderr io.ReadCloser
	process = exec.Command("nats-server", "-c", fp)
	if wantsLog {
		var err error
		stdout, err = process.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		stderr, err = process.StderrPipe()
		if err != nil {
			t.Fatal(err)
		}
	}

	if err := process.Start(); err != nil {
		t.Fatal(err)
	}

	go func() {
		_ = process.Wait()
	}()

	if wantsLog {
		go func() {
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				t.Logf("%s\n", scanner.Text())
			}
		}()

		go func() {
			scanner := bufio.NewScanner(stderr)
			for scanner.Scan() {
				t.Logf("%s\n", scanner.Text())
			}
		}()
	}

	portsFile := filepath.Join(portsFileDir, fmt.Sprintf("nats-server_%d.ports", process.Process.Pid))
	start := time.Now()
	var ports *Ports
	for i := 0; ; i++ {
		if time.Since(start) > 10*time.Second {
			break
		}
		if _, err := os.Stat(portsFile); err == nil {
			d, err := os.ReadFile(portsFile)
			if err == nil {
				var p Ports
				if err := json.Unmarshal(d, &p); err == nil {
					ports = &p
					break
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	if ports == nil {
		t.Fatal("nats-server not started")
	}

	return &ExternalNatsServer{t: t, Process: process, Connections: Connections{
		ConnectionPorts: ConnectionPorts{
			Nats:      ports.Nats,
			WebSocket: ports.WebSocket,
		},
	}}
}

func (es *ExternalNatsServer) Shutdown() {
	es.Connections.Shutdown()
	_ = es.Process.Process.Kill()
}
