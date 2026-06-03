ALTER TABLE students
  DROP CONSTRAINT IF EXISTS students_class_code_format,
  ADD CONSTRAINT students_class_code_format CHECK (class_code IS NULL OR class_code ~ '^(7|8|9|10|11|12)0[1-5]$') NOT VALID,
  DROP CONSTRAINT IF EXISTS students_seat_no_range,
  ADD CONSTRAINT students_seat_no_range CHECK (seat_no_in_class IS NULL OR seat_no_in_class BETWEEN 1 AND 70) NOT VALID;
