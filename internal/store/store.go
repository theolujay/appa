package store

import (
	"database/sql"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	PENDING   string = "pending"
	BUILDING  string = "building"
	DEPLOYING string = "deploying"
	RUNNING   string = "running"
	CANCELED  string = "canceled"
	STOPPED   string = "stopped"
	FAILED    string = "failed"
)

type Store struct {
	db *sql.DB
}

type Deployment struct {
	ID        string  `json:"id"`
	Source    string  `json:"source"`
	Status    string  `json:"status"`
	ImageTag  *string `json:"image_tag"`
	Address   *string `json:"address"`
	EnvVars   *string `json:"env_vars"`
	URL       *string `json:"url"`
	CreatedAt string  `json:"created_at"`
}

type DeploymentUpdate struct {
	Status   *string
	ImageTag *string
	Address  *string
	EnvVars  *string
	URL      *string
}

func New(dsn string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	// enforce foreign keys, as SQLite doesn't do that by default
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, err
	}
	// WAL mode allows conccurent readers alongside a writer.
	// Without this, any read during a write returns SQLITE_BUSY
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, err
	}
	// Wait up to 5 seconds for a lock to clear before returning SQLITE_BUSY
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}

	// Allow multiple connections so WAL mode can handle concurrent readers/writers.
	// This prevents the WebSocket handshake from blocking during heavy build logging.
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)
	return s, nil
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(
		`CREATE TABLE IF NOT EXISTS deployments (
			id			TEXT PRIMARY KEY,
			source		TEXT NOT NULL,
			status		TEXT NOT NULL DEFAULT 'pending',
			image_tag	TEXT,
			address		TEXT,
			env_vars    TEXT,
			url			TEXT,
			created_at	DATETIME NOT NULL DEFAULT (datetime('now'))
		);

		CREATE TABLE IF NOT EXISTS logs (
			id				INTEGER PRIMARY KEY AUTOINCREMENT,
			deployment_id	TEXT NOT NULL REFERENCES deployments(id),
			phase			TEXT NOT NULL,
			line			TEXT NOT NULL,
			ts				DATETIME NOT NULL DEFAULT (datetime('now'))
		);
	`)
	return err
}

func (s *Store) CreateDeployment(id, source string) error {
	_, err := s.db.Exec(
		`INSERT INTO deployments (id, source, status, created_at)
		VALUES (?, ?, 'pending', datetime('now'))`,
		id, source,
	)
	return err
}

// Add New method specifically for deployments with env vars
func (s *Store) CreateDeploymentWithEnv(id, source, envVars string) error {
	_, err := s.db.Exec(
		`INSERT INTO deployments (id, source, status, env_vars, created_at)
		VALUES (?, ?, 'pending', ?, datetime('now'))`,
		id, source, envVars,
	)
	return err
}

func (s *Store) GetDeployment(id string) (Deployment, error) {
	rows, err := s.db.Query(
		`SELECT id, source, status, image_tag, address, env_vars, url, created_at
		FROM deployments WHERE id = ?`, id,
	)
	if err != nil {
		return Deployment{}, err
	}
	defer rows.Close()

	var deployments []Deployment

	for rows.Next() {
		var d Deployment
		err := rows.Scan(&d.ID, &d.Source, &d.Status, &d.ImageTag, &d.Address, &d.EnvVars, &d.URL, &d.CreatedAt)
		if err != nil {
			return Deployment{}, err
		}
		deployments = append(deployments, d)
	}
	if len(deployments) == 0 {
		return Deployment{}, sql.ErrNoRows
	}
	return deployments[0], nil
}

func (s *Store) ListDeployments() ([]Deployment, error) {
	rows, err := s.db.Query(`
        SELECT id, source, status, image_tag, address, env_vars, url, created_at
        FROM deployments
        ORDER BY created_at DESC
    `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deployments []Deployment

	for rows.Next() {
		var d Deployment
		err := rows.Scan(&d.ID, &d.Source, &d.Status, &d.ImageTag, &d.Address, &d.EnvVars, &d.URL, &d.CreatedAt)
		if err != nil {
			return nil, err
		}
		deployments = append(deployments, d)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return deployments, nil
}

type LogEntry struct {
	ID   int64  `json:"id"`
	Line string `json:"line"`
}

func (s *Store) GetLogs(deploymentID string) ([]LogEntry, error) {
	rows, err := s.db.Query(
		`SELECT id, line FROM logs WHERE deployment_id = ? ORDER BY id ASC`,
		deploymentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []LogEntry
	for rows.Next() {
		var l LogEntry
		if err := rows.Scan(&l.ID, &l.Line); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

func (s *Store) AppendLog(deploymentID, phase, line string) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO logs (deployment_id, phase, line) VALUES (?, ?, ?)`,
		deploymentID, phase, line,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateDeployment(id string, u DeploymentUpdate) error {
	query := "UPDATE deployments SET "
	var args []interface{}
	var fields []string

	if u.Status != nil {
		fields = append(fields, "status = ?")
		args = append(args, *u.Status)
	}
	if u.ImageTag != nil {
		fields = append(fields, "image_tag = ?")
		args = append(args, *u.ImageTag)
	}
	if u.Address != nil {
		fields = append(fields, "address = ?")
		args = append(args, *u.Address)
	}
	if u.EnvVars != nil {
		fields = append(fields, "env_vars = ?")
		args = append(args, *u.EnvVars)
	}
	if u.URL != nil {
		fields = append(fields, "url = ?")
		args = append(args, *u.URL)
	}

	if len(fields) == 0 {
		return nil
	}

	query += strings.Join(fields, ", ")
	query += " WHERE id = ?"
	args = append(args, id)

	_, err := s.db.Exec(query, args...)
	return err
}
