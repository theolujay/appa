package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	vd "github.com/theolujay/appa/internal/validator"
)

type DeploymentModel struct {
	DB *sql.DB
}

type Deployment struct {
	ID        int64   `json:"id"`
	UserID    *int64  `json:"user_id"`
	Source    string  `json:"source"`
	Status    string  `json:"status"`
	ImageTag  *string `json:"image_tag"`
	Address   *string `json:"address"`
	EnvVars   *string `json:"env_vars"`
	URL       *string `json:"url"`
	CreatedAt string  `json:"created_at"`
	Version   int
}

// DeploymentUpdate is a container that helps to determine if a column
// should be updated in the database by leveraging a nil pointer.
type DeploymentUpdate struct {
	Status   *string
	ImageTag *string
	Address  *string
	EnvVars  *string
	URL      *string
}

type DeploymentModeler interface {
	Create(d *Deployment) error
	Get(id int64) (*Deployment, error)
	GetAllForUser(id int64, status string, filters Filters) (
		[]Deployment, Metadata, error,
	)
	GetLogs(id int64) ([]LogEntry, error)
	AppendLog(id int64, phase, line string) (int64, error)
	UpdateAndGet(id int64, u DeploymentUpdate) (*Deployment, error)
}

func ValidateDeployment(d *Deployment) error {
	v := vd.New()
	v.Check(d.Source != "", "source", "must be provided")
	v.Check(len(d.Source) <= 500, "source", "must not be more than 500 bytes long")
	if d.EnvVars != nil && *d.EnvVars != "" {
		i := validateEnvVars(*d.EnvVars)
		v.Check(i == 0, "env_vars", fmt.Sprintf("invalid key-value pair at line %d", i))
	}
	if v.Valid() {
		return nil
	}
	return v.Errors
}

func validateEnvVars(e string) int {
	envPairs := strings.Split(e, "\n")
	for i, env := range envPairs {
		kv := strings.Split(env, "=")
		if len(kv) != 2 {
			return i + 1
		}
	}
	return 0
}

func (dm *DeploymentModel) Create(d *Deployment) error {
	query := `
		INSERT INTO deployments (source, env_vars, user_id)
		VALUES($1, $2, $3)
		RETURNING id, status, created_at, version
	`

	err := dm.DB.QueryRow(
		query,
		d.Source,
		d.EnvVars,
		d.UserID,
	).Scan(
		&d.ID,
		&d.Status,
		&d.CreatedAt,
		&d.Version,
	)

	return fmt.Errorf("deployments.create: %w", err)
}

func (dm *DeploymentModel) Get(id int64) (*Deployment, error) {

	query := `
		SELECT id, user_id, source, status, image_tag, address, env_vars, url, created_at
		FROM deployments WHERE id = $1
	`
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var d Deployment

	err := dm.DB.QueryRowContext(ctx, query, id).Scan(
		&d.ID,
		&d.UserID,
		&d.Source,
		&d.Status,
		&d.ImageTag,
		&d.Address,
		&d.EnvVars,
		&d.URL,
		&d.CreatedAt,
	)

	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, ErrRecordNotFound
		default:
			return nil, fmt.Errorf("deployments.get: %w", err)
		}
	}

	return &d, nil
}

func (dm *DeploymentModel) GetAllForUser(
	id int64, status string, filters Filters,
) ([]Deployment, Metadata, error) {
	totalRecords := 0
	md := Metadata{}
	dy := []Deployment{}
	deployments := make([]Deployment, 0, filters.limit())

	query := fmt.Sprintf(`
		SELECT count(*) OVER(), id, user_id, source, status,
			image_tag, address, env_vars, url, created_at, version
        FROM deployments
		WHERE (user_id = $1 OR ($1 = 0 AND user_id IS NULL))
		AND (LOWER(status) = LOWER($2) OR $2 = '')
        ORDER BY %s %s, id ASC
		LIMIT $3 OFFSET $4
	`, filters.sortColumn(), filters.sortDirection())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	args := []any{id, status, filters.limit(), filters.offset()}

	rows, err := dm.DB.QueryContext(ctx, query, args...)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			err = ErrRecordNotFound
		}
		return dy, md, fmt.Errorf("deployments.getAllForUser: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var d Deployment
		err := rows.Scan(
			&totalRecords,
			&d.ID,
			&d.UserID,
			&d.Source,
			&d.Status,
			&d.ImageTag,
			&d.Address,
			&d.EnvVars,
			&d.URL,
			&d.CreatedAt,
			&d.Version,
		)
		if err != nil {
			return dy, md, fmt.Errorf("deployments.getAllForUser: %w", err)
		}
		deployments = append(deployments, d)
	}

	if err := rows.Err(); err != nil {
		return dy, md, fmt.Errorf("deployments.getAllForUser: %w", err)
	}

	md = calculateMetadata(totalRecords, filters.Page, filters.PageSize)

	return deployments, md, nil
}

type LogEntry struct {
	ID   int64  `json:"id"`
	Line string `json:"line"`
}

func (dm *DeploymentModel) GetLogs(id int64) ([]LogEntry, error) {

	query := `SELECT id, line FROM logs WHERE deployment_id = $1 ORDER BY id ASC`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	rows, err := dm.DB.QueryContext(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("deployments.getLogs: %w", err)
	}
	defer rows.Close()

	logs := make([]LogEntry, 0, 64)
	for rows.Next() {
		var l LogEntry
		if err := rows.Scan(&l.ID, &l.Line); err != nil {
			return nil, fmt.Errorf("deployments.getLogs: %w", err)
		}
		logs = append(logs, l)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("deployments.getLogs: %w", rows.Err())
	}
	return logs, nil
}

func (dm *DeploymentModel) AppendLog(id int64, phase, line string) (int64, error) {

	query := `INSERT INTO logs (deployment_id, phase, line) VALUES ($1, $2, $3) RETURNING id`
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var logID int64
	err := dm.DB.QueryRowContext(ctx, query, id, phase, line).Scan(&logID)
	if err != nil {
		return 0, fmt.Errorf("deployments.appendLog: %w", err)
	}
	return logID, nil
}

// UpdateAndGet updates a deployment and returns the full row. It uses a
// RETURNING clause so callers avoid a separate Get round-trip.
func (dm *DeploymentModel) UpdateAndGet(id int64, u DeploymentUpdate) (*Deployment, error) {

	query := "UPDATE deployments SET "
	args := make([]any, 0, 6)
	fields := make([]string, 0, 6)
	dy := Deployment{}

	if u.Status != nil {
		fields = append(fields, fmt.Sprintf("status = $%d", len(args)+1))
		args = append(args, *u.Status)
	}
	if u.ImageTag != nil {
		fields = append(fields, fmt.Sprintf("image_tag = $%d", len(args)+1))
		args = append(args, *u.ImageTag)
	}
	if u.Address != nil {
		fields = append(fields, fmt.Sprintf("address = $%d", len(args)+1))
		args = append(args, *u.Address)
	}
	if u.EnvVars != nil {
		fields = append(fields, fmt.Sprintf("env_vars = $%d", len(args)+1))
		args = append(args, *u.EnvVars)
	}
	if u.URL != nil {
		fields = append(fields, fmt.Sprintf("url = $%d", len(args)+1))
		args = append(args, *u.URL)
	}

	if len(fields) == 0 {
		return &dy, nil
	}

	query += strings.Join(fields, ", ")
	query += fmt.Sprintf(" WHERE id = $%d", len(args)+1)
	query += " RETURNING id, user_id, source, status, image_tag, address, env_vars, url, created_at"
	args = append(args, id)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var d Deployment

	err := dm.DB.QueryRowContext(ctx, query, args...).Scan(
		&d.ID,
		&d.UserID,
		&d.Source,
		&d.Status,
		&d.ImageTag,
		&d.Address,
		&d.EnvVars,
		&d.URL,
		&d.CreatedAt,
	)

	if err != nil {
		return &dy, fmt.Errorf("deployments.updateAndGet: %w", err)
	}
	return &d, nil
}
