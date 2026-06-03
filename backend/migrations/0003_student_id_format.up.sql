ALTER TABLE students
  DROP CONSTRAINT IF EXISTS students_student_id_format,
  ADD CONSTRAINT students_student_id_format CHECK (student_id ~ '^11[0-5][1-4][0-9]{3}$');
