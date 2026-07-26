-- 006_pr_cancel_from_draft — a draft requisition may be cancelled without ever
-- having been submitted.
--
-- §6.10.3 declares:
--
--   CHECK (status = 'draft' OR submitted_at IS NOT NULL)
--
-- which reads as "anything past draft has been submitted". That is true of
-- `submitted`, `approved`, and `rejected`, and false of `cancelled`: §6.9.2 says
-- in as many words that "a draft requisition may be cancelled by its creator",
-- and such a requisition was never submitted. As written the constraint makes
-- that transition impossible, and the only way past it is to stamp
-- `submitted_at` with a submission that did not happen — a lie in the column the
-- status timeline renders from.
--
-- So `cancelled` joins `draft` as a state that does not require the timestamp.
-- Everything else is unchanged: a requisition cancelled *after* submission still
-- carries the real `submitted_at`, because cancelling never clears it.
ALTER TABLE purchase_requisitions
  DROP CONSTRAINT pr_submitted_has_timestamp;

ALTER TABLE purchase_requisitions
  ADD CONSTRAINT pr_submitted_has_timestamp
  CHECK (status IN ('draft', 'cancelled') OR submitted_at IS NOT NULL);
