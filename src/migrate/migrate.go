package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"github.com/spf13/viper"
	"regexp"
	"strconv"
	"github.com/spf13/cobra"
)

var databaseFolder string
var migrationFolder string
var configFolder string
var dbEnvConfig string
var versionFile string

func main () {
	databaseFolder = os.Getenv("GOPATH") + "/database"
	migrationFolder = databaseFolder + "/migration"
	configFolder = os.Getenv("GOPATH") + "/config"
	dbEnvConfig = configFolder + "/database.env.json"
	versionFile = migrationFolder + "/version"

	viper.SetConfigType("json")
	viper.SetConfigName("database")
	viper.AddConfigPath(configFolder)
	viper.ReadInConfig()

	if FileExists(dbEnvConfig) {
		viper.SetConfigName("database.env")
		viper.MergeInConfig()
	}

	// Check current version
	var currentVersion int64
	if !FileExists(versionFile) {
		currentVersion = 0
	} else {
		content, err := ioutil.ReadFile(versionFile)
		if err != nil {
			panic(err)
		}
		strNum := string(content)
		currentVersion, err = strconv.ParseInt(strNum, 10, 64)
		if err != nil {
			panic(err)
		}
	}
	fmt.Println("---------------------")
	fmt.Printf("= Current version %d =\n", currentVersion)
	fmt.Println("---------------------")

	db, err := DbConn()
	if err != nil {
		panic(err)
	}

	defer db.Close()

	var cmdMigrate = &cobra.Command{
		Use:   "up",
		Short: "Start to exexcute sql file by sequence.",
		Run: func(cmd *cobra.Command, args []string) {
			tx, err := db.Begin()
			if err != err {
				panic(err)
			}
			
			defer func () {
				if p := recover(); p != nil {
					tx.Rollback()
					panic(p)
				}
			}()

			files, err := ioutil.ReadDir(migrationFolder)
			if err != nil {
				panic(err)
			}

			re := regexp.MustCompile(`(\d+)\.up\.sql`)
			for _, f := range files {
				match := re.FindAllStringSubmatch(f.Name(), -1)
				if len(match) == 0 {
					continue
				}

				version, err := strconv.ParseInt(match[0][1], 10, 64)
				if err != nil {
					panic(err)
				}

				if version <= currentVersion {
					continue
				}

				fullFilePath := migrationFolder + "/" + f.Name()
				fmt.Println(fullFilePath)

				content, err := ioutil.ReadFile(fullFilePath)
				if err != nil {
					panic(err)
				}

				stmt, err := db.Prepare(string(content))
				if err != nil {
					panic(err)
				}

				result, err := stmt.Exec()
				if err != nil {
					panic(err)
				}

				affected, err := result.RowsAffected()
				if err != nil {
					panic(err)
				}
				fmt.Println("Rows affected %d", affected)
				UpdateVersionNumber(version)
			}

			err = tx.Commit()
		},
	}

	  var rootCmd = &cobra.Command{Use: "migrate"}
	  rootCmd.AddCommand(cmdMigrate)
	  rootCmd.Execute()	
}

func DbConn () (*sql.DB, error) {
	mk := viper.GetString("default")
	dsn := fmt.Sprintf(
			"%s:%s@tcp(%s:%s)/%s?parseTime=True&charset=%s",
			viper.GetString(mk+".username"),
			viper.GetString(mk+".password"),
			viper.GetString(mk+".host"),
			viper.GetString(mk+".port"),
			viper.GetString(mk+".database"),
			viper.GetString(mk+".charset"))
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		fmt.Println(err)
		panic(err)
	}
	return db, err
}

func UpdateVersionNumber(number int64) {
	numberStr := fmt.Sprintf("%d", number)
	if !FileExists(versionFile) {
		f, err := os.OpenFile(versionFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			panic(err)
		}
		defer f.Close()

		if _, err = f.WriteString(numberStr); err != nil {
			panic(err)
		}
		f.Sync()
	} else {
		f, err := os.OpenFile(versionFile, os.O_WRONLY, 0644)
		if err != nil {
			panic(err)
		}
		defer f.Close()

		if _, err = f.WriteString(numberStr); err != nil {
			panic(err)
		}
		f.Sync()
	}
}

func FileExists (path string) bool {
	if _, err := os.Stat(path); err == nil {
		return true
	} else if os.IsNotExist(err) {
		return false	
	}
	return false
}