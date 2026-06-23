package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type Project struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	UserID    *int64    `json:"user_id,omitempty"`
	Version   int       `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

type ProjectModel struct {
	DB *sql.DB
}

type ProjectModeler interface {
	Insert(p *Project) error
	Get(id int64) (*Project, error)
	GetByName(name string) (*Project, error)
	GetAllForUser(id int64, filters Filters) ([]Project, Metadata, error)
	Update(p *Project) error
	Delete(id int64) error
}

func (pm *ProjectModel) Insert(p *Project) error {
	query := `
		INSERT INTO projects (name, user_id)
		VALUES ($1, $2)
		RETURNING id, version, created_at
	`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := pm.DB.QueryRowContext(ctx, query, p.Name, p.UserID).Scan(
		&p.ID,
		&p.Version,
		&p.CreatedAt,
	)
	if err != nil {
		switch {
		case isUniqueViolation(err, pqProjectsNameKey):
			return ErrDuplicateProject
		default:
			return fmt.Errorf("projects.insert: %w", err)
		}
	}
	return nil
}

func (pm *ProjectModel) Get(id int64) (*Project, error) {
	query := `
		SELECT id, name, user_id, version, created_at
		FROM projects WHERE id = $1
	`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var p Project
	err := pm.DB.QueryRowContext(ctx, query, id).Scan(
		&p.ID,
		&p.Name,
		&p.UserID,
		&p.Version,
		&p.CreatedAt,
	)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, ErrRecordNotFound
		default:
			return nil, fmt.Errorf("projects.get: %w", err)
		}
	}
	return &p, nil
}

func (pm *ProjectModel) GetByName(name string) (*Project, error) {
	query := `
		SELECT id, name, user_id, version, created_at
		FROM projects WHERE name = $1
	`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var p Project
	err := pm.DB.QueryRowContext(ctx, query, name).Scan(
		&p.ID,
		&p.Name,
		&p.UserID,
		&p.Version,
		&p.CreatedAt,
	)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, ErrRecordNotFound
		default:
			return nil, fmt.Errorf("projects.getByName: %w", err)
		}
	}
	return &p, nil
}

func (pm *ProjectModel) GetAllForUser(
	id int64, filters Filters,
) ([]Project, Metadata, error) {
	totalRecords := 0
	md := Metadata{}
	projects := make([]Project, 0, filters.limit())

	query := fmt.Sprintf(`
		SELECT count(*) OVER(), id, name, user_id, version, created_at
		FROM projects
		WHERE (user_id = $1 OR ($1 = 0 AND user_id IS NULL))
		ORDER BY %s %s, id ASC
		LIMIT $2 OFFSET $3
	`, filters.sortColumn(), filters.sortDirection())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	args := []any{id, filters.limit(), filters.offset()}

	rows, err := pm.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return projects, md, fmt.Errorf("projects.getAllForUser: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var p Project
		err := rows.Scan(
			&totalRecords,
			&p.ID,
			&p.Name,
			&p.UserID,
			&p.Version,
			&p.CreatedAt,
		)
		if err != nil {
			return projects, md, fmt.Errorf("projects.getAllForUser: %w", err)
		}
		projects = append(projects, p)
	}

	if err := rows.Err(); err != nil {
		return projects, md, fmt.Errorf("projects.getAllForUser: %w", err)
	}

	md = calculateMetadata(totalRecords, filters.Page, filters.PageSize)

	return projects, md, nil
}

func (pm *ProjectModel) Update(p *Project) error {
	query := `
		UPDATE projects
		SET name = $1, user_id = $2, version = version + 1
		WHERE id = $3 AND version = $4
		RETURNING version
	`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := pm.DB.QueryRowContext(ctx, query, p.Name, p.UserID, p.ID, p.Version).Scan(&p.Version)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return ErrEditConflict
		default:
			return fmt.Errorf("projects.update: %w", err)
		}
	}
	return nil
}

func (pm *ProjectModel) Delete(id int64) error {
	query := `
		DELETE FROM projects
		WHERE id = $1
	`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	result, err := pm.DB.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("projects.delete: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("projects.delete: %w", err)
	}

	if rows == 0 {
		return ErrRecordNotFound
	}

	return nil
}
