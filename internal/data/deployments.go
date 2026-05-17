package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/theolujay/appa/internal/validator"
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

type DeploymentModel struct {
	DB *sql.DB
}

type Deployment struct {
	ID        int64   `json:"id"`
	UserID    int64   `json:"user_id"`
	Source    string  `json:"source"`
	Status    string  `json:"status"`
	ImageTag  *string `json:"image_tag"`
	Address   *string `json:"address"`
	EnvVars   *string `json:"env_vars"`
	URL       *string `json:"url"`
	CreatedAt string  `json:"created_at"`
	Version   int
}

type DeploymentUpdate struct {
	Status   *string
	ImageTag *string
	Address  *string
	EnvVars  *string
	URL      *string
}

func ValidateDeployment(v *validator.Validator, d *Deployment) {
	v.Check(d.Source != "", "source", "must be provided")
	v.Check(len(d.Source) <= 500, "source", "must not be more than 500 bytes long")
	if d.EnvVars != nil && *d.EnvVars != "" {
		i := validateEnvVars(*d.EnvVars)
		v.Check(i == 0, "env_vars", fmt.Sprintf("invalid key-value pair at line %d", i))
	}
}

func validateEnvVars(e string) int {
	envPairs := strings.Split(e, "\n")
	for i, env := range envPairs {
		kv := strings.Split(env, "=")
		if len(kv) != 2 {
			return i
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

	return err
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
			return nil, err
		}
	}

	return &d, nil
}

func (dm *DeploymentModel) GetAll(userID int64, status string, filters Filters) ([]*Deployment, Metadata, error) {
	totalRecords := 0
	metadata := Metadata{}
	deployments := []*Deployment{}

	query := fmt.Sprintf(`
		SELECT count(*) OVER(), id, user_id, source, status, image_tag, address, env_vars, url, created_at, version
        FROM deployments
		WHERE (user_id = $1 OR $1 = 0)
		AND (LOWER(status) = LOWER($2) OR $2 = '')
        ORDER BY %s %s, id ASC
		LIMIT $3 OFFSET $4
	`, filters.sortColumn(), filters.sortDirection())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	args := []any{userID, status, filters.limit(), filters.offset()}

	rows, err := dm.DB.QueryContext(ctx, query, args...)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, metadata, err
		}
		return nil, metadata, err
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
			return nil, metadata, err
		}
		deployments = append(deployments, &d)
	}

	if err := rows.Err(); err != nil {
		return nil, metadata, err
	}

	metadata = calculateMetadata(totalRecords, filters.Page, filters.PageSize)

	return deployments, metadata, nil
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

func (dm *DeploymentModel) AppendLog(id int64, phase, line string) (int64, error) {

	query := `INSERT INTO logs (deployment_id, phase, line) VALUES ($1, $2, $3)`
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	res, err := dm.DB.ExecContext(ctx, query, id, phase, line)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (dm *DeploymentModel) Update(id int64, u DeploymentUpdate) error {
	query := "UPDATE deployments SET "
	var args []interface{}
	var fields []string

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
		return nil
	}

	query += strings.Join(fields, ", ")
	query += fmt.Sprintf(" WHERE id = $%d", len(args)+1)
	args = append(args, id)

	_, err := dm.DB.Exec(query, args...)
	return err
}
