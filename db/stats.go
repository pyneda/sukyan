package db

func GetDatabaseSize() (string, error) {
	var result string
	err := Connection.db.Raw("SELECT pg_size_pretty(pg_database_size(current_database()))").Scan(&result).Error
	if err != nil {
		return "", err
	}
	return result, nil
}
