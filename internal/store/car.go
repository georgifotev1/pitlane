package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var ErrDuplicatePlate = errors.New("duplicate plate")

type CarStore struct {
	db *DB
}

func NewCarStore(db *DB) *CarStore {
	return &CarStore{db: db}
}

const carColumns = "id, tenant_id, customer_id, plate, vin, make, model, year, mileage, archived_at, created_at, updated_at"

func scanCar(row pgx.Row) (*domain.Car, error) {
	var c domain.Car
	if err := row.Scan(
		&c.ID, &c.TenantID, &c.CustomerID, &c.Plate, &c.VIN, &c.Make,
		&c.Model, &c.Year, &c.Mileage, &c.ArchivedAt, &c.CreatedAt, &c.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &c, nil
}

type CarListParams struct {
	Search          string
	IncludeArchived bool
	Limit           int
	Offset          int
}

func (s *CarStore) List(ctx context.Context, tenantID, customerID string, p CarListParams) ([]*domain.Car, int, error) {
	var cars []*domain.Car
	var total int

	err := s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		const where = `
			WHERE tenant_id = $1
			  AND customer_id = $2
			  AND ($3 = '' OR plate ILIKE '%' || $3 || '%'
			                OR vin ILIKE '%' || $3 || '%'
			                OR make ILIKE '%' || $3 || '%'
			                OR model ILIKE '%' || $3 || '%')
			  AND ($4 OR archived_at IS NULL)`

		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM cars`+where,
			tenantID, customerID, p.Search, p.IncludeArchived,
		).Scan(&total); err != nil {
			return fmt.Errorf("count cars: %w", err)
		}

		rows, err := tx.Query(ctx,
			`SELECT `+carColumns+` FROM cars`+where+`
			 ORDER BY plate ASC, id ASC
			 LIMIT $5 OFFSET $6`,
			tenantID, customerID, p.Search, p.IncludeArchived, p.Limit, p.Offset,
		)
		if err != nil {
			return fmt.Errorf("list cars: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			c, err := scanCar(rows)
			if err != nil {
				return fmt.Errorf("scan car: %w", err)
			}
			cars = append(cars, c)
		}
		return rows.Err()
	})
	return cars, total, err
}

type CarSummary struct {
	Car          *domain.Car
	CustomerName string
}

func (s *CarStore) ListAll(ctx context.Context, tenantID string, p CarListParams) ([]CarSummary, int, error) {
	var out []CarSummary
	var total int

	err := s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		const where = `
			WHERE c.tenant_id = $1
			  AND ($2 = '' OR c.plate ILIKE '%' || $2 || '%'
			                OR c.vin ILIKE '%' || $2 || '%'
			                OR c.make ILIKE '%' || $2 || '%'
			                OR c.model ILIKE '%' || $2 || '%')
			  AND ($3 OR c.archived_at IS NULL)`

		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM cars c `+where,
			tenantID, p.Search, p.IncludeArchived,
		).Scan(&total); err != nil {
			return fmt.Errorf("count cars: %w", err)
		}

		rows, err := tx.Query(ctx,
			`SELECT c.id, c.tenant_id, c.customer_id, c.plate, c.vin, c.make, c.model,
			        c.year, c.mileage, c.archived_at, c.created_at, c.updated_at,
			        cust.name
			 FROM cars c
			 JOIN customers cust ON cust.id = c.customer_id AND cust.tenant_id = c.tenant_id
			 `+where+`
			 ORDER BY c.plate ASC, c.id ASC
			 LIMIT $4 OFFSET $5`,
			tenantID, p.Search, p.IncludeArchived, p.Limit, p.Offset,
		)
		if err != nil {
			return fmt.Errorf("list cars board: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var c domain.Car
			var customerName string
			if err := rows.Scan(
				&c.ID, &c.TenantID, &c.CustomerID, &c.Plate, &c.VIN, &c.Make, &c.Model,
				&c.Year, &c.Mileage, &c.ArchivedAt, &c.CreatedAt, &c.UpdatedAt,
				&customerName,
			); err != nil {
				return fmt.Errorf("scan car summary: %w", err)
			}
			out = append(out, CarSummary{Car: &c, CustomerName: customerName})
		}
		return rows.Err()
	})
	return out, total, err
}

func (s *CarStore) Get(ctx context.Context, tenantID, id string) (*domain.Car, error) {
	var car *domain.Car
	err := s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT `+carColumns+` FROM cars WHERE id = $1 AND tenant_id = $2`,
			id, tenantID,
		)
		c, err := scanCar(row)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		car = c
		return nil
	})
	return car, err
}

func (s *CarStore) Create(ctx context.Context, c *domain.Car) error {
	return s.db.WithTenant(ctx, c.TenantID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			INSERT INTO cars (id, tenant_id, customer_id, plate, vin, make, model, year, mileage)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING created_at, updated_at
		`, c.ID, c.TenantID, c.CustomerID, c.Plate, c.VIN, c.Make, c.Model, c.Year, c.Mileage).
			Scan(&c.CreatedAt, &c.UpdatedAt)
		return mapCarWriteError(err)
	})
}

func (s *CarStore) Update(ctx context.Context, c *domain.Car) error {
	return s.db.WithTenant(ctx, c.TenantID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			UPDATE cars
			SET plate = $3, vin = $4, make = $5, model = $6, year = $7, mileage = $8, updated_at = now()
			WHERE id = $1 AND tenant_id = $2
			RETURNING updated_at
		`, c.ID, c.TenantID, c.Plate, c.VIN, c.Make, c.Model, c.Year, c.Mileage)
		if err := row.Scan(&c.UpdatedAt); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return mapCarWriteError(err)
		}
		return nil
	})
}

func (s *CarStore) Archive(ctx context.Context, tenantID, id string) error {
	return s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE cars
			SET archived_at = now(), updated_at = now()
			WHERE id = $1 AND tenant_id = $2 AND archived_at IS NULL
		`, id, tenantID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
}

func mapCarWriteError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrDuplicatePlate
	}
	return err
}
