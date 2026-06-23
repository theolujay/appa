package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type ProjectEnvVar struct {
	ID        int64     `json:"id"`
	ProjectID int64     `json:"project_id"`
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	Version   int       `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

type ProjectEnvVarModel struct {
	DB *sql.DB
}

type ProjectEnvVarModeler interface {
	Upsert(projectID int64, key, value string) error
	UpsertMany(projectID int64, envs map[string]string) error
	GetAll(projectID int64) ([]ProjectEnvVar, error)
	GetByKey(projectID int64, key string) (*ProjectEnvVar, error)
	Delete(projectID int64, key string) error
}

// Upsert updates an env vars or a project. If it already exists for the project,
// it updates its value and bumps the version; otherwise it inserts it fresh and
// the version starts at the default, 1.
func (m *ProjectEnvVarModel) Upsert(projectID int64, key, value string) error {
	query := `
		INSERT INTO project_envs (project_id, key, value)
		VALUES ($1, $2, $3)
		ON CONFLICT (project_id, key)
		DO UPDATE SET value = EXCLUDED.value, version = project_envs.version + 1
		RETURNING id, version, created_at
	`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var ev ProjectEnvVar
	err := m.DB.QueryRowContext(ctx, query, projectID, key, value).Scan(
		&ev.ID, &ev.Version, &ev.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("projectEnvVar.upsert: %w", err)
	}
	return nil
}

// UpsertMany runs Upsert over each key-value pair in envs. It batches all errors
// encountered and returns them all at once.
func (m *ProjectEnvVarModel) UpsertMany(projectID int64, envs map[string]string) error {
	var errs []error
	for key, value := range envs {
		if err := m.Upsert(projectID, key, value); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func (m *ProjectEnvVarModel) GetAll(projectID int64) ([]ProjectEnvVar, error) {
	query := `
		SELECT id, project_id, key, value, version, created_at
		FROM project_envs
		WHERE project_id = $1
		ORDER BY key ASC
	`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	rows, err := m.DB.QueryContext(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("projectEnvVar.getAll: %w", err)
	}
	defer rows.Close()

	var envs []ProjectEnvVar
	for rows.Next() {
		var ev ProjectEnvVar
		if err := rows.Scan(&ev.ID, &ev.ProjectID, &ev.Key, &ev.Value, &ev.Version, &ev.CreatedAt); err != nil {
			return nil, fmt.Errorf("projectEnvVar.getAll: %w", err)
		}
		envs = append(envs, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("projectEnvVar.getAll: %w", err)
	}
	return envs, nil
}

func (m *ProjectEnvVarModel) GetByKey(projectID int64, key string) (*ProjectEnvVar, error) {
	query := `
		SELECT id, project_id, key, value, version, created_at
		FROM project_envs
		WHERE project_id = $1 AND key = $2
	`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var ev ProjectEnvVar
	err := m.DB.QueryRowContext(ctx, query, projectID, key).Scan(
		&ev.ID, &ev.ProjectID, &ev.Key, &ev.Value, &ev.Version, &ev.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("projectEnvVar.getByKey: %w", err)
	}
	return &ev, nil
}

func (m *ProjectEnvVarModel) Delete(projectID int64, key string) error {
	query := `
		DELETE FROM project_envs
		WHERE project_id = $1 AND key = $2
	`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	result, err := m.DB.ExecContext(ctx, query, projectID, key)
	if err != nil {
		return fmt.Errorf("projectEnvVar.delete: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("projectEnvVar.delete: %w", err)
	}
	if rows == 0 {
		return ErrRecordNotFound
	}
	return nil
}

// MergeProjectEnvVars returns the merged env vars string: project-level vars
// merged with deployment-level vars. Deployment-level vars override project vars
// on key conflict.
func MergeProjectEnvVars(projectEnvVars []ProjectEnvVar, deploymentEnvVars *string) *string {
	merged := make(map[string]string)

	for _, ev := range projectEnvVars {
		merged[ev.Key] = ev.Value
	}

	if deploymentEnvVars != nil && *deploymentEnvVars != "" {
		lines := strings.Split(*deploymentEnvVars, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			kv := strings.SplitN(line, "=", 2)
			if len(kv) == 2 {
				merged[kv[0]] = kv[1]
			}
		}
	}

	if len(merged) == 0 {
		return nil
	}

	var b strings.Builder
	first := true
	for k, v := range merged {
		if !first {
			b.WriteByte('\n')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(v)
		first = false
	}
	result := b.String()
	return &result
}
