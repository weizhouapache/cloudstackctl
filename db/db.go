package db

import (
	v1 "cloudstackctl/apis/v1"
	"fmt"
	"log"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// DB is the global PostgreSQL connection
var DB *gorm.DB

// Init initializes the PostgreSQL connection and migrates schema
func Init() error {
	// Prefer a full DSN from `DATABASE_DSN` if provided, otherwise assemble
	// from individual `PG*` environment variables with sensible defaults.
	dsn := os.Getenv("DATABASE_DSN")
	if dsn == "" {
		host := getEnv("PGHOST", "localhost")
		user := getEnv("PGUSER", "postgres")
		pass := getEnv("PGPASSWORD", "secret")
		dbname := getEnv("PGDATABASE", "cloudstackctl")
		port := getEnv("PGPORT", "5432")
		ssl := getEnv("PGSSLMODE", "disable")
		dsn = fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s", host, user, pass, dbname, port, ssl)
	}

	// Open database connection
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Printf("Failed to connect to PostgreSQL: %v", err)
		return err
	}

	// Auto-migrate only controller-managed resource schemas. CloudStack-managed
	// resources (Network, Volume, SSHKey, SecurityGroup, AffinityGroup, UserData)
	// are not persisted in the DB as they are managed directly via the SDK.
	err = db.AutoMigrate(
		&v1.Application{},
		&v1.Component{},
		&v1.VirtualMachineSpecResource{},
		&v1.VirtualMachine{},
	)
	if err != nil {
		log.Printf("Failed to migrate database schema: %v", err)
		return err
	}

	DB = db

	// Ensure commonly queried indexes exist (metadata.name)
	// GORM stores embedded metadata fields with column names like metadata_name
	if !db.Migrator().HasIndex(&v1.Application{}, "metadata_name") {
		db.Migrator().CreateIndex(&v1.Application{}, "metadata_name")
	}
	if !db.Migrator().HasIndex(&v1.Component{}, "metadata_name") {
		db.Migrator().CreateIndex(&v1.Component{}, "metadata_name")
	}
	if !db.Migrator().HasIndex(&v1.VirtualMachine{}, "metadata_name") {
		db.Migrator().CreateIndex(&v1.VirtualMachine{}, "metadata_name")
	}
	if !db.Migrator().HasIndex(&v1.VirtualMachineSpecResource{}, "metadata_name") {
		db.Migrator().CreateIndex(&v1.VirtualMachineSpecResource{}, "metadata_name")
	}

	log.Println("PostgreSQL initialized successfully")
	return nil
}

// getEnv returns the env var value or the provided default
func getEnv(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}
