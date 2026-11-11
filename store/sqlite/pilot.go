package sqlite

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mattn/go-sqlite3"
	"github.com/mtlynch/logpaste/store"
)

const (
	pilotPageSize  = 4096
	pilotMaxPages  = 2048
	pilotMainBytes = pilotPageSize * pilotMaxPages
	pilotAppID     = 0x4c50564e // LPVN in the SQLite header.
	pilotVersion   = 1
	pilotFile      = "store.db"
)

type Pilot struct {
	db
	mu   sync.Mutex
	conn *sql.Conn
	pool *sql.DB
}

// OpenPilot owns one pinned connection, so no request can silently lose the
// per-connection page limit or use another journal mode.
func OpenPilot(dir string) (*Pilot, error) {
	path := filepath.Join(dir, pilotFile)
	fresh, err := inspectPilotFiles(dir)
	if err != nil {
		return nil, err
	}
	if fresh {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return nil, errors.New("cannot create the pilot database on the mounted volume")
		}
		if file.Close() != nil {
			return nil, errors.New("cannot close the new pilot database file")
		}
	}

	pool, err := sql.Open("sqlite3", "file:"+path+"?mode=rw&_busy_timeout=3000&_synchronous=FULL")
	if err != nil {
		return nil, errors.New("cannot open the pilot SQLite database")
	}
	pool.SetMaxOpenConns(1)
	ctx := context.Background()
	conn, err := pool.Conn(ctx)
	if err != nil {
		pool.Close()
		return nil, errors.New("cannot connect to the pilot SQLite database")
	}
	pilot := &Pilot{db: db{ctx: conn, retention: 24 * time.Hour}, conn: conn, pool: pool}
	if err := pilot.configure(ctx, path, fresh); err != nil {
		pilot.Close()
		return nil, err
	}
	return pilot, nil
}

func inspectPilotFiles(dir string) (bool, error) {
	volume, err := os.Open(dir)
	if err != nil {
		return false, errors.New("cannot inspect the pilot data volume")
	}
	defer volume.Close()
	// At most the database and its journal belong on this dedicated volume.
	files, err := volume.ReadDir(3)
	if err != nil && err != io.EOF {
		return false, errors.New("cannot inspect the pilot data volume")
	}
	if len(files) == 0 {
		return true, nil
	}
	foundDB := false
	for _, file := range files {
		info, err := file.Info()
		if err != nil || !info.Mode().IsRegular() {
			return false, errors.New("the pilot data volume contains an unexpected file")
		}
		switch file.Name() {
		case pilotFile:
			foundDB = info.Size() <= pilotMainBytes
		case pilotFile + "-journal":
			if info.Size() > 24*1024*1024 {
				return false, errors.New("the existing pilot journal exceeds the startup limit")
			}
		default:
			return false, errors.New("use a dedicated volume with an empty or existing pilot database")
		}
	}
	if !foundDB || !hasPilotHeader(filepath.Join(dir, pilotFile)) {
		return false, errors.New("the existing database is incompatible with the pilot")
	}
	return false, nil
}

// Read the header directly. Opening an unknown database with SQLite could
// recover its journal before we can decide whether the volume belongs to us.
func hasPilotHeader(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	var header [100]byte
	if _, err := io.ReadFull(file, header[:]); err != nil {
		return false
	}
	return string(header[:16]) == "SQLite format 3\x00" &&
		binary.BigEndian.Uint16(header[16:18]) == pilotPageSize &&
		header[18] == 1 && header[19] == 1 &&
		binary.BigEndian.Uint32(header[60:64]) == pilotVersion &&
		binary.BigEndian.Uint32(header[68:72]) == pilotAppID
}

func (p *Pilot) configure(ctx context.Context, path string, fresh bool) error {
	var journal string
	if err := p.conn.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journal); err != nil || journal != "delete" {
		return errors.New("the pilot database requires SQLite DELETE journaling")
	}
	if fresh {
		if _, err := p.conn.ExecContext(ctx, fmt.Sprintf("PRAGMA page_size=%d", pilotPageSize)); err != nil {
			return errors.New("cannot configure the pilot SQLite page size")
		}
	}
	if _, err := p.conn.ExecContext(ctx, "PRAGMA cache_spill=OFF; PRAGMA temp_store=MEMORY"); err != nil {
		return errors.New("cannot configure the pilot SQLite connection")
	}
	var pageSize, maxPages, spill int
	if err := p.conn.QueryRowContext(ctx, "PRAGMA cache_spill").Scan(&spill); err != nil || spill != 0 {
		return errors.New("cannot disable pilot SQLite cache spill")
	}
	if err := p.conn.QueryRowContext(ctx, "PRAGMA page_size").Scan(&pageSize); err != nil || pageSize != pilotPageSize {
		return errors.New("unexpected pilot SQLite page size")
	}
	limit := fmt.Sprintf("PRAGMA max_page_count=%d", pilotMaxPages)
	if err := p.conn.QueryRowContext(ctx, limit).Scan(&maxPages); err != nil || maxPages != pilotMaxPages {
		return errors.New("cannot enforce the pilot SQLite page budget")
	}
	if fresh && (p.initialize(ctx) != nil || !hasPilotHeader(path)) {
		return errors.New("cannot initialize the pilot SQLite database")
	}
	var unexpected int
	if err := p.conn.QueryRowContext(ctx, `
		SELECT count(*) FROM sqlite_schema
		WHERE name NOT GLOB 'sqlite_*' AND NOT (name='entries' AND type='table')`).Scan(&unexpected); err != nil || unexpected != 0 {
		return errors.New("the pilot database contains an incompatible schema")
	}
	probe := p.conn.QueryRowContext(ctx, "SELECT id, creation_time, contents FROM entries LIMIT 0")
	var id, created, contents string
	if err := probe.Scan(&id, &created, &contents); err != sql.ErrNoRows {
		return errors.New("the pilot database is missing the expected entries table")
	}
	if p.PurgeExpired() != nil {
		return errors.New("cannot check and expire rows on the pilot data volume")
	}
	return nil
}

func (p *Pilot) initialize(ctx context.Context) error {
	schema, err := migrationsFs.ReadFile("migrations/001-create-entries-table.sql")
	if err != nil {
		return err
	}
	tx, err := p.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, string(schema)); err != nil {
		return err
	}
	identity := fmt.Sprintf("PRAGMA application_id=%d; PRAGMA user_version=%d", pilotAppID, pilotVersion)
	if _, err := tx.ExecContext(ctx, identity); err != nil {
		return err
	}
	return tx.Commit()
}

func (d db) expiryCutoff() string {
	return time.Now().UTC().Add(-d.retention).Format(time.RFC3339)
}

func (p *Pilot) PurgeExpired() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, err := p.conn.ExecContext(context.Background(), "DELETE FROM entries WHERE creation_time<=?", p.expiryCutoff())
	return err
}

func (p *Pilot) GetEntry(id string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.db.GetEntry(id)
}

func (p *Pilot) InsertEntry(id, contents string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	err := p.db.InsertEntry(id, contents)
	var sqliteErr sqlite3.Error
	if errors.As(err, &sqliteErr) && sqliteErr.Code == sqlite3.ErrFull {
		return store.ErrCapacity
	}
	return err
}

func (p *Pilot) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.conn.Close()
	p.pool.Close()
}
