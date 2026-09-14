-- The first migration. It creates nothing your brief needs — the tables are
-- yours to design.
--
-- `create table if not exists` and a name-ordered runner mean `make migrate` is
-- safe to re-run, which is what you want while you are still changing the
-- schema.
create table if not exists schema_notes (
  applied_at timestamptz not null default now(),
  note       text        not null
);

insert into schema_notes (note) values ('0001_init: skeleton only, no application tables yet');
