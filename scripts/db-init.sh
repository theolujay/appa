#!/bin/bash
set -e


psql -v ON_ERROR_STOP=1 --username postgres <<-EOSQL
  CREATE DATABASE appa;
  CREATE EXTENSION IF NOT EXISTS citext;
  CREATE ROLE appa WITH LOGIN PASSWORD 'pa55word';
  ALTER DATABASE appa OWNER TO appa;
EOSQL