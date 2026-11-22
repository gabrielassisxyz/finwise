package authored

// allStatements registers every SQL statement the package executes, whichever
// file declares it, so that one test can prepare all of them against the
// migrated schema. A constant named statement… that is missing here, or a
// statement executed that is not one of these, fails a test.
var allStatements = map[string]string{
	"statementStampApplicationID":    statementStampApplicationID,
	"statementMigrationsTableExists": statementMigrationsTableExists,
	"statementRecordedMigrations":    statementRecordedMigrations,
	"statementCreateMigrationsTable": statementCreateMigrationsTable,
	"statementRecordMigration":       statementRecordMigration,
	"statementCopyInto":              statementCopyInto,
	"statementRecordTrackingStart":   statementRecordTrackingStart,
	"statementTrackingStart":         statementTrackingStart,
	"statementRecordLastBackup":      statementRecordLastBackup,
	"statementLastBackup":            statementLastBackup,
	"statementApplicationID":         statementApplicationID,
	"statementIntegrityCheck":        statementIntegrityCheck,
}
