# Username Generation

Student usernames in Boddle follow the format `{firstName}{lastInitial}{number}`, e.g. `christians1`, `christians2`. This service generates unique usernames by tracking the highest assigned number per base prefix in PostgreSQL.

## How It Works

1. **Build the base** — lowercase the student's first name (letters only) + first letter of last name.
   - `"Christian"`, `"St. John"` → `christians`
   - `"Jo-Anne"`, `"Lee"` → `joannel`
   - Non-letter characters (spaces, hyphens, apostrophes, digits) are stripped.

2. **Truncate** — the base is capped at 14 characters to leave room for at least a 1-digit number within the 15-character username limit.

3. **Assign the next number** — an atomic Postgres upsert increments `max_number` for that base and returns the new value. The first student with a given base gets number `1`.

4. **Fit within 15 chars** — if the number needs more digits (e.g. `10`, `100`), the base is further truncated so `len(base) + len(number) <= 15`.

### Examples

| First Name | Last Name | Base (key) | Number | Username |
|---|---|---|---|---|
| Christian | Smith | `christians` | 1 | `christians1` |
| Christian | Smith | `christians` | 2 | `christians2` |
| Anna | Lee | `annal` | 1 | `annal1` |
| ChristianJames | Smith | `christianjames` → truncated to `christianjame` (14) | 1 | `christianjame1` |
| ChristianJames | Smith | `christianjame` | 10 | `christianjam10` |

## Database Schema

```sql
CREATE TABLE username_sequences (
    base_username VARCHAR(14) NOT NULL PRIMARY KEY,
    max_number    INTEGER     NOT NULL DEFAULT 0
);
```

The `NextNumber` operation is a single atomic statement:

```sql
INSERT INTO username_sequences (base_username, max_number)
VALUES ($1, 1)
ON CONFLICT (base_username)
DO UPDATE SET max_number = username_sequences.max_number + 1
RETURNING max_number;
```

This is safe under concurrent requests — Postgres row-level locking ensures no two callers receive the same number.

## Migration

The migration at `migrations/001_create_username_sequences.sql` creates the table and seeds it from existing student usernames. It extracts the alphabetic prefix and max numeric suffix from every row in the `students` table so that new usernames won't collide with existing ones.

## Package Structure

```
internal/username/
├── repository.go      # DB operations (NextNumber, CurrentNumber)
├── service.go         # Username generation logic (Generate, BuildBase)
└── service_test.go    # Unit tests
```

## Usage

```go
usernameRepo := username.NewRepository(db.DB)
usernameService := username.NewService(usernameRepo)

name, err := usernameService.Generate(ctx, "Christian", "St. John")
// name = "christians1" (or next available number)
```

## Differences from the Legacy Ruby/Redis Implementation

The Rails LMS previously tracked usernames in Redis using a range-based system (see `UsernamesHelper` in the Rails codebase).

| | Legacy (Ruby + Redis) | Current (Go + Postgres) |
|---|---|---|
| **Storage** | Redis hash (`lms.usernames`) | Postgres table (`username_sequences`) |
| **Tracking** | Ranges of taken numbers (e.g. `[[1,5],[8,8]]`) | Single `max_number` per base |
| **Assignment** | First gap in ranges | Always `max_number + 1` |
| **Number reuse** | Yes (fills gaps from deleted usernames) | No (monotonically increasing) |
| **Concurrency** | Redis single-threaded | Postgres row-level locking |

The trade-off is that numbers are never recycled, but the implementation is simpler and the sequence state lives alongside the rest of the application data in Postgres rather than in a separate Redis store.
