// SPDX-License-Identifier: AGPL-3.0-only
package testutil

import (
	"context"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func StartEphemeralPG(t *testing.T) string {
	t.Helper()
	if base := os.Getenv("CHASTEDESK_TEST_DATABASE_URL"); base != "" {
		return startTestDatabase(t, base)
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	name := fmt.Sprintf("chastedesk-test-%08x", rand.Uint32())
	run := exec.Command("docker", "run", "-d", "--rm", "--name", name,
		"-e", "POSTGRES_USER=chastedesk",
		"-e", "POSTGRES_PASSWORD=chastedesk",
		"-e", "POSTGRES_DB=chastedesk",
		"-p", "127.0.0.1::5432",
		"pgvector/pgvector:pg16")
	out, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("docker run: %v: %s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", name).Run()
	})
	hostPort := ""
	for i := 0; i < 40 && hostPort == ""; i++ {
		portOut, err := exec.Command("docker", "port", name, "5432").Output()
		if err == nil && len(portOut) > 0 {
			line := strings.TrimSpace(strings.Split(string(portOut), "\n")[0])
			if idx := strings.LastIndex(line, ":"); idx >= 0 {
				hostPort = line[idx+1:]
			}
		}
		if hostPort == "" {
			time.Sleep(250 * time.Millisecond)
		}
	}
	if hostPort == "" {
		t.Fatal("could not determine mapped port")
	}
	dsn := fmt.Sprintf("postgresql://chastedesk:chastedesk@127.0.0.1:%s/chastedesk", hostPort)
	deadline := time.Now().Add(120 * time.Second)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		conn, err := pgx.Connect(ctx, dsn)
		if err == nil {
			err = conn.Ping(ctx)
			_ = conn.Close(ctx)
		}
		cancel()
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("postgres not ready: %v", err)
		}
		time.Sleep(500 * time.Millisecond)
	}
	return dsn
}

func startTestDatabase(t *testing.T, base string) string {
	t.Helper()
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" {
		t.Fatalf("CHASTEDESK_TEST_DATABASE_URL is not a valid postgres URL")
	}
	name := fmt.Sprintf("chastedesk_test_%d_%08x", time.Now().UnixNano(), rand.Uint32())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatalf("connect to test database server: %v", err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	if _, err := admin.Exec(ctx, `create database "`+name+`"`); err != nil {
		t.Fatalf("create test database: %v", err)
	}
	t.Cleanup(func() {
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer dropCancel()
		conn, err := pgx.Connect(dropCtx, base)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close(context.Background()) }()
		_, _ = conn.Exec(dropCtx, `drop database if exists "`+name+`" with (force)`)
	})
	parsed.Path = "/" + name
	return parsed.String()
}
