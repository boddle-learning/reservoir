-- Create username_sequences table for tracking the max assigned number
-- per base username (firstName + lastInitial, lowercased, truncated to 14 chars).
-- Used by the username generation service to assign the next sequential number.

CREATE TABLE IF NOT EXISTS username_sequences (
    base_username VARCHAR(14) NOT NULL PRIMARY KEY,
    max_number    INTEGER     NOT NULL DEFAULT 0
);

-- Add a UNIQUE constraint on students.username as a DB-level safety net against
-- duplicate usernames. The application layer prevents duplicates via the atomic
-- upsert + IsUsernameTaken retry loop, but this constraint is the final backstop.
-- NOTE: CONCURRENTLY must be executed outside a transaction block.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_students_username_unique
    ON students (username)
    WHERE username IS NOT NULL AND username != '';

-- Seed from existing student usernames.
-- Extracts the alphabetic prefix, normalizes it to the same lowercase
-- 14-character base username format used by the generator, and finds the
-- max numeric suffix per normalized prefix.
INSERT INTO username_sequences (base_username, max_number)
SELECT
    prefix,
    MAX(num) AS max_number
FROM (
    SELECT
        LEFT(LOWER(regexp_replace(username, '[0-9]+$', '')), 14) AS prefix,
        COALESCE(NULLIF(regexp_replace(username, '^[^0-9]*', ''), ''), '0')::integer AS num
    FROM students
    WHERE username IS NOT NULL AND username != ''
) sub
WHERE prefix != ''
GROUP BY prefix
ON CONFLICT (base_username) DO UPDATE
    SET max_number = GREATEST(username_sequences.max_number, EXCLUDED.max_number);
