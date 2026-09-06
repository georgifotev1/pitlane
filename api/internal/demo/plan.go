package demo

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/gfotev/pitlane/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// monthlyJobs is how many cars the demonstration garage finishes each month,
// oldest month first - roughly one to two a working day, which is what a small
// two-ramp workshop actually turns over. It climbs gently, with the usual
// August dip: the numbers are invented, but the shape should not be more
// dramatic than a real workshop's.
var monthlyJobs = [months]int{24, 27, 22, 29, 26, 31, 28, 33, 30, 25, 34, 36}

type outcome int

const (
	outcomeCompleted outcome = iota
	outcomeOpen
	outcomeInProgress
	outcomeQuoteOnly
)

type plannedJob struct {
	Vehicle int
	Job     int
	Mileage int
	// QuotedAt is when the customer got the quote; StampedAt is when the work
	// finished, or when the document last moved for anything unfinished.
	QuotedAt  time.Time
	StampedAt time.Time
	Outcome   outcome
	// QuoteStatus applies to outcomeQuoteOnly: the state the offer ended in.
	QuoteStatus domain.OfferStatus
}

type plan struct {
	now  time.Time
	jobs []plannedJob
}

func newPlan(now time.Time) *plan {
	rng := randomiser()
	p := &plan{now: now}

	// Cars are driven at different rates, which is what makes the odometer on
	// each job believable rather than uniform.
	annualKm := make([]int, len(vehicles))
	for i := range vehicles {
		annualKm[i] = 12000 + rng.IntN(5)*4000
	}
	mileageAt := func(v int, at time.Time) int {
		years := now.Sub(at).Hours() / (24 * 365)
		km := vehicles[v].Mileage - int(years*float64(annualKm[v]))
		if km < 1000 {
			km = 1000 + rng.IntN(500)
		}
		return km
	}

	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	for i := range months {
		start := monthStart.AddDate(0, -(months - 1 - i), 0)
		count := monthlyJobs[i]
		if i == months-1 {
			// The current month is only as busy as the part of it that has
			// happened; a full month of work on the 3rd would give the trend a
			// false last column. The dashboard compares like with like, so a
			// part-built month reads correctly rather than as a collapse.
			count = count * now.Day() / daysIn(start)
			if count < 1 {
				count = 1
			}
		}
		for k := range count {
			day := 1 + (k*26)/max(count, 1) + rng.IntN(2)
			if day > daysIn(start) {
				day = daysIn(start)
			}
			done := start.AddDate(0, 0, day-1).Add(time.Duration(9+rng.IntN(8)) * time.Hour)
			if done.After(now) {
				done = now.Add(-2 * time.Hour)
			}
			v := rng.IntN(len(vehicles))
			p.jobs = append(p.jobs, plannedJob{
				Vehicle:   v,
				Job:       weightedJob(rng),
				Mileage:   mileageAt(v, done),
				QuotedAt:  done.AddDate(0, 0, -(1 + rng.IntN(4))),
				StampedAt: done,
				Outcome:   outcomeCompleted,
			})
		}

		// Not every quote turns into work. These keep the acceptance rate on the
		// dashboard honest instead of a flattering 100%.
		for range 4 + rng.IntN(5) {
			day := 3 + rng.IntN(24)
			if day > daysIn(start) {
				day = daysIn(start)
			}
			quoted := start.AddDate(0, 0, day-1).Add(11 * time.Hour)
			if quoted.After(now) {
				continue
			}
			v := rng.IntN(len(vehicles))
			status := domain.OfferStatusRejected
			if rng.IntN(3) == 0 {
				status = domain.OfferStatusExpired
			}
			p.jobs = append(p.jobs, plannedJob{
				Vehicle:     v,
				Job:         weightedJob(rng),
				Mileage:     mileageAt(v, quoted),
				QuotedAt:    quoted,
				StampedAt:   quoted,
				Outcome:     outcomeQuoteOnly,
				QuoteStatus: status,
			})
		}
	}

	// Work currently on the ramps, including one job that has not moved in over
	// a month so the "needs attention" panel has something real to show.
	inProgress := []struct {
		vehicle, job, daysAgo int
		outcome               outcome
	}{
		{3, 4, 2, outcomeInProgress},
		{7, 3, 6, outcomeOpen},
		{10, 5, 34, outcomeOpen},
	}
	for _, w := range inProgress {
		started := now.AddDate(0, 0, -w.daysAgo)
		p.jobs = append(p.jobs, plannedJob{
			Vehicle:   w.vehicle,
			Job:       w.job,
			Mileage:   mileageAt(w.vehicle, started),
			QuotedAt:  started.AddDate(0, 0, -2),
			StampedAt: started,
			Outcome:   w.outcome,
		})
	}

	// Quotes still waiting for an answer; the older two are past the fortnight
	// the dashboard treats as stale.
	waiting := []struct {
		vehicle, job, daysAgo int
		status                domain.OfferStatus
	}{
		{1, 2, 23, domain.OfferStatusSent},
		{5, 9, 17, domain.OfferStatusSent},
		{8, 1, 4, domain.OfferStatusSent},
		{11, 6, 2, domain.OfferStatusDraft},
	}
	for _, w := range waiting {
		quoted := now.AddDate(0, 0, -w.daysAgo)
		p.jobs = append(p.jobs, plannedJob{
			Vehicle:     w.vehicle,
			Job:         w.job,
			Mileage:     mileageAt(w.vehicle, quoted),
			QuotedAt:    quoted,
			StampedAt:   quoted,
			Outcome:     outcomeQuoteOnly,
			QuoteStatus: w.status,
		})
	}
	return p
}

// weightedJob favours the routine work a garage actually repeats - servicing,
// brakes, diagnostics - over the once-in-a-lifetime clutch replacement.
func weightedJob(rng *rand.Rand) int {
	weights := []int{9, 8, 5, 2, 2, 4, 7, 4, 5, 4, 2, 6}
	total := 0
	for _, w := range weights {
		total += w
	}
	pick := rng.IntN(total)
	for i, w := range weights {
		if pick < w {
			return i
		}
		pick -= w
	}
	return 0
}

func daysIn(month time.Time) int {
	return time.Date(month.Year(), month.Month()+1, 0, 0, 0, 0, 0, month.Location()).Day()
}

// stamp is a document whose timestamps have to be moved into the past once the
// stores have created it the ordinary way.
type stamp struct {
	table     string
	id        string
	createdAt time.Time
	stampedAt time.Time
	status    domain.OfferStatus
}

func (p *plan) execute(ctx context.Context, s stores, tenantID string, result *Result) error {
	customerIDs, carIDs, err := p.createFleet(ctx, s, tenantID, result)
	if err != nil {
		return err
	}

	var stamps []stamp
	for _, planned := range p.jobs {
		offer := &domain.Offer{
			ID:         uuid.NewString(),
			TenantID:   tenantID,
			CarID:      carIDs[planned.Vehicle],
			TaxRateBps: domain.StandardVATRateBPS,
			Notes:      "Офертата важи 30 дни. Цените са с включено ДДС.",
			Items:      offerItems(jobs[planned.Job]),
		}
		if err := s.offers.Create(ctx, offer); err != nil {
			return fmt.Errorf("create demo offer: %w", err)
		}
		result.Offers++
		stamps = append(stamps, stamp{table: "offers", id: offer.ID, createdAt: planned.QuotedAt, stampedAt: planned.QuotedAt})

		if planned.Outcome == outcomeQuoteOnly {
			stamps[len(stamps)-1].status = planned.QuoteStatus
			continue
		}

		repair, err := s.repairs.CreateFromOffer(ctx, tenantID, offer.ID)
		if err != nil {
			return fmt.Errorf("accept demo offer: %w", err)
		}
		result.Repairs++

		switch planned.Outcome {
		case outcomeCompleted:
			if _, err := s.repairs.Complete(ctx, tenantID, repair.ID, planned.Mileage); err != nil {
				return fmt.Errorf("complete demo repair: %w", err)
			}
		case outcomeInProgress:
			if _, err := s.repairs.SetStatus(ctx, tenantID, repair.ID, domain.RepairStatusInProgress); err != nil {
				return fmt.Errorf("start demo repair: %w", err)
			}
		}
		stamps = append(stamps, stamp{table: "repairs", id: repair.ID, createdAt: planned.StampedAt.AddDate(0, 0, -1), stampedAt: planned.StampedAt})
	}

	return p.backdate(ctx, s.db, tenantID, stamps, customerIDs, carIDs)
}

func (p *plan) createFleet(ctx context.Context, s stores, tenantID string, result *Result) ([]string, []string, error) {
	customerIDs := make([]string, len(customers))
	for i, c := range customers {
		customer := &domain.Customer{
			ID: uuid.NewString(), TenantID: tenantID,
			Name: c.Name, Company: c.Company, Email: c.Email, Phone: c.Phone, Address: c.Address,
		}
		if err := s.customers.Create(ctx, customer); err != nil {
			return nil, nil, fmt.Errorf("create demo customer: %w", err)
		}
		customerIDs[i] = customer.ID
		result.Customers++
	}

	carIDs := make([]string, len(vehicles))
	for i, v := range vehicles {
		car := &domain.Car{
			ID: uuid.NewString(), TenantID: tenantID, CustomerID: customerIDs[v.Customer],
			Plate: v.Plate, VIN: v.VIN, Make: v.Make, Model: v.Model, Year: v.Year,
		}
		if err := s.cars.Create(ctx, car); err != nil {
			return nil, nil, fmt.Errorf("create demo car: %w", err)
		}
		carIDs[i] = car.ID
		result.Cars++
	}

	for _, n := range serviceNotes {
		recorded := p.now.AddDate(0, -n.MonthsAgo, 0)
		note := &domain.HistoryNote{
			ID: uuid.NewString(), TenantID: tenantID, CarID: carIDs[n.Vehicle],
			Title: n.Title, Description: n.Description, RecordedAt: recorded,
		}
		if err := s.history.CreateNote(ctx, note); err != nil {
			return nil, nil, fmt.Errorf("create demo history note: %w", err)
		}
	}
	return customerIDs, carIDs, nil
}

func offerItems(j job) []domain.OfferItem {
	items := make([]domain.OfferItem, 0, len(j.Parts)+1)
	for _, part := range j.Parts {
		items = append(items, domain.OfferItem{
			Kind:           domain.OfferItemKindPart,
			Description:    part.Description,
			Quantity:       1,
			UnitPriceCents: part.PriceCents,
			CostCents:      part.CostCents,
		})
	}
	items = append(items, domain.OfferItem{
		Kind:           domain.OfferItemKindLabor,
		Description:    j.Title,
		Quantity:       1,
		UnitPriceCents: j.LaborCents,
	})
	return items
}

// backdate moves the finished documents into the past. Nothing in the
// application can do this, and nothing should: it is the one thing a fixture
// needs that a garage never does.
func (p *plan) backdate(ctx context.Context, db *store.DB, tenantID string, stamps []stamp, customerIDs, carIDs []string) error {
	earliest := p.now
	for _, st := range stamps {
		if st.createdAt.Before(earliest) {
			earliest = st.createdAt
		}
	}
	registered := earliest.AddDate(0, -1, 0)

	return db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		for _, st := range stamps {
			switch st.table {
			case "offers":
				if _, err := tx.Exec(ctx, `
					UPDATE offers
					SET created_at = $2, updated_at = $3,
					    status = COALESCE(NULLIF($4, ''), status),
					    send_status = CASE WHEN $4 = 'sent' THEN 'sent' ELSE send_status END,
					    sent_at = CASE WHEN $4 = 'sent' THEN $2 ELSE sent_at END,
					    sent_to = CASE WHEN $4 = 'sent' THEN (
					        SELECT cust.email FROM cars c
					        JOIN customers cust ON cust.id = c.customer_id AND cust.tenant_id = c.tenant_id
					        WHERE c.id = offers.car_id AND c.tenant_id = offers.tenant_id
					    ) ELSE sent_to END
					WHERE id = $1 AND tenant_id = $5
				`, st.id, st.createdAt, st.stampedAt, string(st.status), tenantID); err != nil {
					return fmt.Errorf("backdate demo offer: %w", err)
				}
			case "repairs":
				if _, err := tx.Exec(ctx, `
					UPDATE repairs
					SET created_at = $2, updated_at = $3,
					    completed_at = CASE WHEN status = 'completed' THEN $3 ELSE completed_at END
					WHERE id = $1 AND tenant_id = $4
				`, st.id, st.createdAt, st.stampedAt, tenantID); err != nil {
					return fmt.Errorf("backdate demo repair: %w", err)
				}
			}
		}

		for _, id := range customerIDs {
			if _, err := tx.Exec(ctx, `UPDATE customers SET created_at = $2, updated_at = $2 WHERE id = $1 AND tenant_id = $3`, id, registered, tenantID); err != nil {
				return fmt.Errorf("backdate demo customer: %w", err)
			}
		}
		for _, id := range carIDs {
			if _, err := tx.Exec(ctx, `UPDATE cars SET created_at = $2, updated_at = $2 WHERE id = $1 AND tenant_id = $3`, id, registered, tenantID); err != nil {
				return fmt.Errorf("backdate demo car: %w", err)
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE tenants SET created_at = $2 WHERE id = $1`, tenantID, registered); err != nil {
			return fmt.Errorf("backdate demo tenant: %w", err)
		}

		return renumberDocuments(ctx, tx, tenantID)
	})
}

// renumberDocuments re-issues the annual document numbers now that the
// documents have their real dates, so a repair finished last November is not
// numbered as if it were written this year. The numbers move through a unique
// temporary value first: the sequences restart each year, so assigning the
// final numbers directly would collide with the ones still in place.
func renumberDocuments(ctx context.Context, tx pgx.Tx, tenantID string) error {
	for _, doc := range []struct{ table, prefix string }{{"offers", "OF"}, {"repairs", "RP"}} {
		// Year 9999 is the staging ground: it satisfies the document-number
		// format check and cannot collide with a number any real year issues.
		if _, err := tx.Exec(ctx, `
			WITH staged AS (
			    SELECT id, row_number() OVER (ORDER BY id) AS seq
			    FROM `+doc.table+`
			    WHERE tenant_id = $1
			)
			UPDATE `+doc.table+` d
			SET document_number = $2 || '-9999-' || lpad(staged.seq::text, 6, '0')
			FROM staged
			WHERE staged.id = d.id AND d.tenant_id = $1
		`, tenantID, doc.prefix); err != nil {
			return fmt.Errorf("stage %s numbers: %w", doc.table, err)
		}
		if _, err := tx.Exec(ctx, `
			WITH ranked AS (
			    SELECT id,
			           $2 || '-' || EXTRACT(YEAR FROM created_at)::integer || '-' ||
			               lpad(row_number() OVER (
			                   PARTITION BY EXTRACT(YEAR FROM created_at)
			                   ORDER BY created_at, id
			               )::text, 6, '0') AS document_number
			    FROM `+doc.table+`
			    WHERE tenant_id = $1
			)
			UPDATE `+doc.table+` d
			SET document_number = ranked.document_number
			FROM ranked
			WHERE ranked.id = d.id AND d.tenant_id = $1
		`, tenantID, doc.prefix); err != nil {
			return fmt.Errorf("renumber %s: %w", doc.table, err)
		}
	}

	// The counters have to agree with what was just issued, or the first real
	// document created after a demonstration would reuse a number.
	if _, err := tx.Exec(ctx, `DELETE FROM document_counters WHERE tenant_id = $1`, tenantID); err != nil {
		return fmt.Errorf("reset document counters: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO document_counters (tenant_id, document_type, year, last_number)
		SELECT $1, 'offer', EXTRACT(YEAR FROM created_at)::integer, count(*)
		FROM offers WHERE tenant_id = $1 GROUP BY 3
		UNION ALL
		SELECT $1, 'repair', EXTRACT(YEAR FROM created_at)::integer, count(*)
		FROM repairs WHERE tenant_id = $1 GROUP BY 3
	`, tenantID); err != nil {
		return fmt.Errorf("rebuild document counters: %w", err)
	}
	return nil
}
