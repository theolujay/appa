package store

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func New(dsn string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA forerign_keys = ON"); err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) CreateDeployment(id, source string) error {
	_, err := s.db.Exec(
		`INSERT INTO deployments (id, source, status, created_at)
		VALUES (?, ?, 'pending', datetime('now'))`,
		id, source,
	)
	return err
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(
		`CREATE TABLE IF NOT EXISTS deployments (
			id			TEXT PRIMARY KEY,
			source		TEXT NOT NULL,
			status		TEXT NOT NULL DEFAULT 'pending',
			image_tag	TEXT,
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

func (s *Store) GetLogs(deploymentID string) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT line FROM TABLE logs WHERE deployment_id = ? ORDER BY ts ASC`,
		deploymentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return nil, err
		}
		logs = append(logs, line)
	}
	return logs, rows.Err()
}

func (s *Store) UpdateDeploymentStatus(deploymentID, status string) error {
	_, err := s.db.Exec(
		`UPDATE deployments SET status = ? WHERE id = ?`,
		status, deploymentID,
	)
	return err
}

func (s *Store) AppendLog(deploymentID, phase, line string) error {
	_, err := s.db.Exec(
		`INSERT INTO logs (deployment_id, phase, line) VALUES (?, ?, ?)`,
		deploymentID, phase, line,
	)
	return err
}
