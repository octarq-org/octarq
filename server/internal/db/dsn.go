package db

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// PostgresConfig holds parsed connection parameters from a Postgres DSN.
type PostgresConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// MySQLConfig holds parsed connection parameters from a MySQL DSN.
type MySQLConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	Protocol string
	Socket   string
}

// ParseMySQLDSN parses a MySQL DSN string in either URL format
// (mysql://user:pass@host:port/dbname) or Go DSN format
// (user:pass@tcp(host:port)/dbname?charset=utf8mb4&parseTime=True&loc=Local).
func ParseMySQLDSN(dsn string) (MySQLConfig, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return MySQLConfig{}, fmt.Errorf("empty DSN")
	}

	if strings.HasPrefix(dsn, "mysql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return MySQLConfig{}, fmt.Errorf("parse mysql URL: %w", err)
		}
		host := u.Hostname()
		if host == "" {
			host = "127.0.0.1"
		}
		port := u.Port()
		if port == "" {
			port = "3306"
		}
		pass, _ := u.User.Password()
		dbname := strings.TrimPrefix(u.Path, "/")
		if dbname == "" {
			return MySQLConfig{}, fmt.Errorf("missing dbname in DSN")
		}

		return MySQLConfig{
			Host:     host,
			Port:     port,
			User:     u.User.Username(),
			Password: pass,
			DBName:   dbname,
			Protocol: "tcp",
		}, nil
	}

	// Standard Go MySQL DSN: [user[:password]@][protocol[(address)]]/dbname[?param1=value1&...]
	withoutParams := dsn
	if idx := strings.Index(withoutParams, "?"); idx != -1 {
		withoutParams = withoutParams[:idx]
	}

	slashIdx := strings.LastIndex(withoutParams, "/")
	if slashIdx == -1 || slashIdx == len(withoutParams)-1 {
		return MySQLConfig{}, fmt.Errorf("missing dbname in DSN")
	}
	dbname := withoutParams[slashIdx+1:]
	prefix := withoutParams[:slashIdx]

	var user, pass, protoAddr string
	if atIdx := strings.LastIndex(prefix, "@"); atIdx != -1 {
		userInfo := prefix[:atIdx]
		protoAddr = prefix[atIdx+1:]
		user, pass, _ = strings.Cut(userInfo, ":")
	} else {
		protoAddr = prefix
	}

	cfg := MySQLConfig{
		User:     user,
		Password: pass,
		DBName:   dbname,
		Host:     "127.0.0.1",
		Port:     "3306",
		Protocol: "tcp",
	}

	if strings.HasPrefix(protoAddr, "unix(") && strings.HasSuffix(protoAddr, ")") {
		cfg.Protocol = "unix"
		cfg.Socket = protoAddr[5 : len(protoAddr)-1]
		cfg.Host = ""
		cfg.Port = ""
		return cfg, nil
	}

	if strings.HasPrefix(protoAddr, "tcp(") && strings.HasSuffix(protoAddr, ")") {
		addr := protoAddr[4 : len(protoAddr)-1]
		if h, p, err := net.SplitHostPort(addr); err == nil {
			cfg.Host = h
			cfg.Port = p
		} else if addr != "" {
			cfg.Host = addr
		}
		return cfg, nil
	}

	if protoAddr != "" && protoAddr != "tcp" {
		if h, p, err := net.SplitHostPort(protoAddr); err == nil {
			cfg.Host = h
			cfg.Port = p
		} else {
			cfg.Host = protoAddr
		}
	}

	return cfg, nil
}

// ParsePostgresDSN parses a Postgres DSN string in either URL format
// (postgres://user:pass@host:port/dbname?sslmode=...) or key-value format
// (host=localhost port=5432 user=postgres password=secret dbname=octarq).
func ParsePostgresDSN(dsn string) (PostgresConfig, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return PostgresConfig{}, fmt.Errorf("empty DSN")
	}

	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return PostgresConfig{}, fmt.Errorf("parse postgres URL: %w", err)
		}
		host := u.Hostname()
		if host == "" {
			host = "localhost"
		}
		port := u.Port()
		if port == "" {
			port = "5432"
		}
		pass, _ := u.User.Password()
		dbname := strings.TrimPrefix(u.Path, "/")
		sslmode := u.Query().Get("sslmode")

		return PostgresConfig{
			Host:     host,
			Port:     port,
			User:     u.User.Username(),
			Password: pass,
			DBName:   dbname,
			SSLMode:  sslmode,
		}, nil
	}

	// Key-value format: host=localhost port=5432 user=postgres password=secret dbname=octarq
	cfg := PostgresConfig{Host: "localhost", Port: "5432"}
	parts := strings.Fields(dsn)
	for _, part := range parts {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		k = strings.ToLower(strings.TrimSpace(k))
		v = strings.TrimSpace(v)
		v = strings.Trim(v, "'\"")
		switch k {
		case "host":
			cfg.Host = v
		case "port":
			cfg.Port = v
		case "user":
			cfg.User = v
		case "password":
			cfg.Password = v
		case "dbname":
			cfg.DBName = v
		case "sslmode":
			cfg.SSLMode = v
		}
	}
	if cfg.DBName == "" {
		return PostgresConfig{}, fmt.Errorf("missing dbname in DSN")
	}
	return cfg, nil
}
