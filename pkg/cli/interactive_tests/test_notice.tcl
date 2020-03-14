#! /usr/bin/env expect -f

source [file join [file dirname $argv0] common.tcl]

# This test ensures notices are being sent as expected.

spawn $argv demo --empty
eexpect root@

start_test "Test that notices always appear at the end after all results."
send "SELECT IF(@1=4,crdb_internal.notice('hello'),@1) AS MYRES FROM generate_series(1,10);\r"
eexpect myres
eexpect 1
eexpect 10
eexpect "10 rows"
eexpect "NOTICE: hello"
eexpect root@

# Ditto with multiple result sets. Notices after all result sets.
send "SELECT crdb_internal.notice('hello') AS STAGE1;"
send "SELECT crdb_internal.notice('world') AS STAGE2;\r"
eexpect stage1
eexpect stage2
eexpect "NOTICE: hello"
eexpect "NOTICE: world"
eexpect root@
end_test

start_test "Test creating a view without TEMP set on a TEMP TABLE notifies it will become a TEMP VIEW."
send "SET experimental_enable_temp_tables = true; CREATE TEMP TABLE temp_view_test_tbl(a int); CREATE VIEW temp_view_test AS SELECT a FROM temp_view_test_tbl;\r"
eexpect "NOTICE: view \"temp_view_test\" will be a temporary view"
end_test

start_test "Check dropping an index prints a message about GC TTL."
send "CREATE TABLE drop_index_test(a int); CREATE INDEX drop_index_test_index ON drop_index_test(a); DROP INDEX drop_index_test_index;\r"
eexpect "NOTICE: index \"drop_index_test_index\" will be dropped asynchronously and will be complete after the GC TTL"
end_test

start_test "Check that altering the primary key of a table prints a message about async jobs."
send "CREATE TABLE alterpk (x INT NOT NULL); ALTER TABLE alterpk ALTER PRIMARY KEY USING COLUMNS (x);\r"
eexpect "NOTICE: primary key changes spawn async cleanup jobs. Future schema changes on \"alterpk\" may be delayed as these jobs finish"
end_test

send "\\q\r"
eexpect eof
