-- KAI-283: Rollback LPR watchlist schema.

-- postgres-only:begin
DROP TABLE IF EXISTS lpr_reads;
-- postgres-only:end

DROP TABLE IF EXISTS lpr_watchlist_entries;
DROP TABLE IF EXISTS lpr_watchlists;
