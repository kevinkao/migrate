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
	"strings"
	"github.com/manifoldco/promptui"
	"errors"
)

var databaseFolder string
var migrationFolder string
var configFolder string
var dbEnvConfig string
var versionFile string
var currentVersion int64

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

	var cmdUp = &cobra.Command{
		Use:   "up",
		Short: "Start to exexcute sql file by sequence.",
		Run: func(cmd *cobra.Command, args []string) {
			validate := func(input string) error {
				match, err := regexp.MatchString(`^[YyNn]{1}`, input)
				if err != nil {
					panic(err)
				}
				if (!match) {
					return errors.New("Wrong answer")
				}
				return nil
			}

			prompt := promptui.Prompt{
				Label: "Are you sure? (Y/n)",
				Validate: validate,
			}

			result, err := prompt.Run()

			if err != nil {
				fmt.Printf("Prompt failed %v\n", err)
				return
			}

			if (result == "N" || result == "n") {
				// Stop execute!
				return
			}

			RunMigrate()
		},
	}

	var cmdRollback = &cobra.Command{
		Use:   "down",
		Short: "Rollback migraion",
		Run: func(cmd *cobra.Command, args []string) {
			validate := func(input string) error {
				match, err := regexp.MatchString(`^[YyNn]{1}`, input)
				if err != nil {
					panic(err)
				}
				if (!match) {
					return errors.New("Wrong answer")
				}
				return nil
			}

			prompt := promptui.Prompt{
				Label: "Are you sure? (Y/n)",
				Validate: validate,
			}

			result, err := prompt.Run()

			if err != nil {
				fmt.Printf("Prompt failed %v\n", err)
				return
			}

			if (result == "N" || result == "n") {
				// Stop execute!
				return
			}

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

			for v := currentVersion; v > 0; v-- {
				downfile := fmt.Sprintf("%s/%d.down.sql", migrationFolder, v)
				if !FileExists(downfile) {
					panic(fmt.Sprintf("No such %s", downfile))
				}
				fmt.Println(downfile)

				content, err := ioutil.ReadFile(downfile)
				if err != nil {
					panic(err)
				}

				result, err := db.Exec(string(content))
				if err != nil {
					panic(err)
				}

				if _, err := result.RowsAffected(); err != nil {
					panic(err)
				}
				UpdateVersionNumber(v)
			}

			if err := os.Remove(versionFile); err != nil {
				panic(err)
			}

			err = tx.Commit()
		},
	}

	var cmdFresh = &cobra.Command{
		Use:   "fresh",
		Short: "Drop all tables and run up sql files",
		Run: func(cmd *cobra.Command, args []string) {
			validate := func(input string) error {
				match, err := regexp.MatchString(`^[YyNn]{1}`, input)
				if err != nil {
					panic(err)
				}
				if (!match) {
					return errors.New("Wrong answer")
				}
				return nil
			}

			prompt := promptui.Prompt{
				Label: "Are you sure? (Y/n)",
				Validate: validate,
			}

			result, err := prompt.Run()

			if err != nil {
				fmt.Printf("Prompt failed %v\n", err)
				return
			}

			if (result == "N" || result == "n") {
				// Stop execute!
				return
			}

			stmt, err := db.Prepare("SELECT table_name FROM information_schema.tables WHERE table_schema = ?")
			if err != nil {
				panic(err)
			}

			rows, err := stmt.Query(GetConfig("database"))
			if err != nil {
				panic(err)
			}

			var tables []string
			for rows.Next() {
				var tableName string
				if err := rows.Scan(&tableName); err != nil {
					fmt.Println(err)
				}
				tables = append(tables, tableName)
			}

			tablesStr := strings.Join(tables, ", ")
			fmt.Printf("Drop tables [%s]\n", tablesStr)
			db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", tablesStr))

			if FileExists(versionFile) {
				if err := os.Remove(versionFile); err != nil {
					panic(err)
				}
			}

			currentVersion = 0
			RunMigrate()
		},
	}

	var rootCmd = &cobra.Command{Use: "migrate"}
	rootCmd.AddCommand(cmdUp)
	rootCmd.AddCommand(cmdRollback)
	rootCmd.AddCommand(cmdFresh)
	rootCmd.Execute()	
}

func RunMigrate () {
	db, err := DbConn()
	if err != nil {
		panic(err)
	}

	defer db.Close()

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

		result, err := db.Exec(string(content))
		if err != nil {
			panic(err)
		}

		if _, err := result.RowsAffected(); err != nil {
			panic(err)
		}
		UpdateVersionNumber(version)
	}
	err = tx.Commit()
}

func DbConn () (*sql.DB, error) {
	mk := viper.GetString("default")
	dsn := fmt.Sprintf(
			"%s:%s@tcp(%s:%s)/%s?parseTime=true&multiStatements=true&charset=%s",
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

func GetConfig (key string, defaultValue ...interface {}) (interface {}) {
	mk := viper.GetString("default")
	k := fmt.Sprintf(mk+".%s", key)
	viper.SetDefault(k, defaultValue)
	return viper.GetString(k)
}

func UpdateVersionNumber(number int64) {
	numberStr := fmt.Sprintf("%d", number)
	if !FileExists(versionFile) {
		fmt.Println("create version file")
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
