-- PostgreSQL dump/restore distributes varchar[] casts over array elements.
-- Recreate these checks from the declared migration expressions so the
-- migration runner can compare the restored legacy-103 schema exactly.
-- Each ADD CONSTRAINT validates all existing rows within this transaction.
BEGIN;
ALTER TABLE evaluation_datasets DROP CONSTRAINT evaluation_datasets_scope_check,
  ADD CONSTRAINT evaluation_datasets_scope_check CHECK (scope IN ('system', 'tenant'));
ALTER TABLE evaluation_question_results DROP CONSTRAINT evaluation_question_results_status_check,
  ADD CONSTRAINT evaluation_question_results_status_check CHECK (status IN ('success', 'failed', 'canceled'));
ALTER TABLE im_channels DROP CONSTRAINT chk_im_channels_session_mode,
  ADD CONSTRAINT chk_im_channels_session_mode CHECK (session_mode IN ('user', 'thread'));
ALTER TABLE model_call_records DROP CONSTRAINT model_call_records_status_check,
  ADD CONSTRAINT model_call_records_status_check CHECK (status IN ('started', 'success', 'error', 'canceled'));
COMMIT;
