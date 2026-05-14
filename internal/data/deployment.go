package data

import (
	"database/sql"
	"strings"

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

	v.Check(d.Status != "", "status", "must be provided")
	v.Check(validator.PermittedValue(d.Status, PENDING, BUILDING, DEPLOYING, RUNNING, CANCELED, STOPPED, FAILED), "status", "must be a valid status")
}

func (dm *DeploymentModel) CreateDeployment(d *Deployment) error {
	query := `
		INSERT INTO deployments (source, env_vars)
		VALUES($1, $2)
		RETURNING id, status, created_at, version
	`

	return dm.DB.QueryRow(query, d.Source, d.EnvVars).Scan(&d.ID, &d.Status, &d.CreatedAt, &d.Version)
}

func (s *DeploymentModel) GetDeployment(id int64) (Deployment, error) {
	rows, err := s.DB.Query(
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

func (s *DeploymentModel) ListDeployments() ([]Deployment, error) {
	rows, err := s.DB.Query(`
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

func (s *DeploymentModel) GetLogs(deploymentID int64) ([]LogEntry, error) {
	rows, err := s.DB.Query(
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

func (s *DeploymentModel) AppendLog(deploymentID int64, phase, line string) (int64, error) {
	res, err := s.DB.Exec(
		`INSERT INTO logs (deployment_id, phase, line) VALUES (?, ?, ?)`,
		deploymentID, phase, line,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *DeploymentModel) UpdateDeployment(id int64, u DeploymentUpdate) error {
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

	_, err := s.DB.Exec(query, args...)
	return err
}
