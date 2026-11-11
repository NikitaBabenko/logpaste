package sqlite

import (
	"context"
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/mtlynch/logpaste/store"
)

type queries interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type db struct {
	ctx       queries
	retention time.Duration
}

func New(path string) store.Store {
	ensureDirExists(filepath.Dir(path))
	ctx, err := sql.Open("sqlite3", path)
	if err != nil {
		log.Fatalln(err)
	}

	if _, err := ctx.Exec(`
-- Apply Litestream recommendations: https://litestream.io/tips/
PRAGMA busy_timeout = 5000;
PRAGMA synchronous = NORMAL;
PRAGMA journal_mode = WAL;
PRAGMA wal_autocheckpoint = 0;
		`); err != nil {
		log.Fatalf("failed to set pragmas: %v", err)
	}

	applyMigrations(ctx)

	return &db{
		ctx: ctx,
	}
}

func (d db) GetEntry(id string) (string, error) {
	var contents string
	query := "SELECT contents FROM entries WHERE id=?"
	args := []any{id}
	if d.retention > 0 {
		query += " AND creation_time>?"
		args = append(args, d.expiryCutoff())
	}
	if err := d.ctx.QueryRowContext(context.Background(), query, args...).Scan(&contents); err != nil {
		if err == sql.ErrNoRows {
			return "", store.EntryNotFoundError{ID: id}
		}
		return "", err
	}
	return contents, nil
}

func (d db) InsertEntry(id string, contents string) error {
	now := time.Now()
	if d.retention > 0 {
		now = now.UTC()
	}
	_, err := d.ctx.ExecContext(context.Background(), `
	INSERT INTO entries(
		id,
		creation_time,
		contents)
	values(?,?,?)`, id, now.Format(time.RFC3339), contents)
	return err
}

func ensureDirExists(dir string) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.Mkdir(dir, os.ModePerm); err != nil {
			panic(err)
		}
	}
}
