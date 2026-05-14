package data

import (
	"context"
	"database/sql"
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
	if *d.EnvVars != "" {
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

func (dm *DeploymentModel) CreateDeployment(d *Deployment) error {
	query := `
		INSERT INTO deployments (source, env_vars)
		VALUES($1, $2)
		RETURNING id, status, created_at, version
	`

	return dm.DB.QueryRow(query, d.Source, d.EnvVars).Scan(&d.ID, &d.Status, &d.CreatedAt, &d.Version)
}

func (dm *DeploymentModel) GetDeployment(id int64) (Deployment, error) {

	query := `
		SELECT id, source, status, image_tag, address, env_vars, url, created_at
		FROM deployments WHERE id = $1
	`
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var d Deployment

	err := dm.DB.QueryRowContext(ctx, query, id).Scan(
		&d.ID,
		&d.Source,
		&d.Status,
		&d.ImageTag,
		&d.Address,
		&d.EnvVars,
		&d.URL,
		&d.CreatedAt,
	)

	if err != nil {
		return Deployment{}, err
	}

	return d, nil
}

func (dm *DeploymentModel) ListDeployments() ([]Deployment, error) {
	query := `
		SELECT id, source, status, image_tag, address, env_vars, url, created_at
        FROM deployments
        ORDER BY created_at DESC
	`
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	rows, err := dm.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deployments []Deployment

	for rows.Next() {
		var d Deployment
		err := rows.Scan(
			&d.ID,
			&d.Source,
			&d.Status,
			&d.ImageTag,
			&d.Address,
			&d.EnvVars,
			&d.URL,
			&d.CreatedAt,
		)
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

func (dm *DeploymentModel) GetLogs(deploymentID int64) ([]LogEntry, error) {

	query := `SELECT id, line FROM logs WHERE deployment_id = $1 ORDER BY id ASC`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	rows, err := dm.DB.QueryContext(ctx, query, deploymentID)
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

func (dm *DeploymentModel) AppendLog(deploymentID int64, phase, line string) (int64, error) {

	query := `INSERT INTO logs (deployment_id, phase, line) VALUES ($1, $2, $3)`
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	res, err := dm.DB.ExecContext(ctx, query, deploymentID, phase, line)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (dm *DeploymentModel) UpdateDeployment(id int64, u DeploymentUpdate) error {
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
