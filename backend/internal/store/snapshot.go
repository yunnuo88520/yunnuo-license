package store

import (
	"context"
	"fmt"
	mysql "github.com/go-sql-driver/mysql"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// SnapshotDatabase makes a transactionally consistent logical snapshot. The
// caller coordinates release files separately; it never copies a live DB file.
func (s *Store) SnapshotDatabase(ctx context.Context, path string) error {
	if s.driver == "sqlite" {
		_, err := s.db.ExecContext(ctx, "VACUUM INTO '"+strings.ReplaceAll(path, "'", "''")+"'")
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	return MySQLCommand(ctx, s.dsn, "mysqldump", nil, f, "--single-transaction", "--skip-lock-tables", "--no-tablespaces", "--set-gtid-purged=OFF", "--hex-blob")
}

// Credentials travel through a private defaults file, never the process list.
func MySQLCommand(ctx context.Context, dsn, program string, input io.Reader, output io.Writer, extra ...string) error {
	c, err := mysql.ParseDSN(dsn)
	if err != nil {
		return fmt.Errorf("invalid MySQL DSN")
	}
	escape := func(v string) string {
		return "\"" + strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\n", "\\n", "\r", "\\r").Replace(v) + "\""
	}
	options := "[client]\nuser=" + escape(c.User) + "\npassword=" + escape(c.Passwd) + "\n"
	if c.Net == "unix" {
		options += "protocol=socket\nsocket=" + escape(c.Addr) + "\n"
	} else {
		host, port, err := net.SplitHostPort(c.Addr)
		if err != nil {
			return err
		}
		if _, err = strconv.Atoi(port); err != nil {
			return err
		}
		options += "protocol=tcp\nhost=" + escape(host) + "\nport=" + port + "\n"
	}
	switch c.TLSConfig {
	case "", "false":
		options += "ssl-mode=DISABLED\n"
	case "preferred":
		options += "ssl-mode=PREFERRED\n"
	case "skip-verify":
		options += "ssl-mode=REQUIRED\n"
	case "true":
		ca := ""
		for _, path := range []string{os.Getenv("SSL_CERT_FILE"), "/etc/ssl/certs/ca-certificates.crt", "/etc/pki/tls/certs/ca-bundle.crt", "/etc/ssl/cert.pem"} {
			if path != "" {
				if info, e := os.Stat(path); e == nil && info.Mode().IsRegular() {
					ca = path
					break
				}
			}
		}
		if ca == "" {
			return fmt.Errorf("a system CA bundle is required for verified MySQL TLS snapshots")
		}
		options += "ssl-mode=VERIFY_IDENTITY\nssl-ca=" + escape(ca) + "\n"
	default:
		return fmt.Errorf("custom MySQL TLS registration needs an operator-managed database snapshot")
	}
	tmp, err := os.MkdirTemp("", "yunnuo-mysql-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	optionPath := filepath.Join(tmp, "client.cnf")
	if err = os.WriteFile(optionPath, []byte(options), 0600); err != nil {
		return err
	}
	args := append([]string{"--defaults-extra-file=" + optionPath}, extra...)
	args = append(args, "--", c.DBName)
	cmd := exec.CommandContext(ctx, program, args...)
	cmd.Stdin = input
	cmd.Stdout = output
	// Server diagnostics can contain data. Return an operation-level error only.
	if err = cmd.Run(); err != nil {
		return fmt.Errorf("%s failed (check local database access and client compatibility): %w", program, err)
	}
	return nil
}
func (s *Store) DatabaseEmpty(ctx context.Context) (bool, error) {
	if s.driver != "mysql" {
		return false, fmt.Errorf("empty check requires MySQL")
	}
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE()`).Scan(&count)
	return count == 0, err
}
