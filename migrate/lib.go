package main

import (
	"database/sql"
	"github.com/manifoldco/promptui"
	"errors"
	"regexp"
)

func WithTransaction (db *sql.DB, fn func(tx *sql.Tx) (err error)) (err error) {
	tx, err := db.Begin()
	if err != nil {
		return
	}

	defer func() {
		if p := recover(); p != nil {
			// a panic occurred, rollback and repanic
			tx.Rollback()
			panic(p)
		} else if err != nil {
			// something went wrong, rollback
			tx.Rollback()
		} else {
			// all good, commit
			err = tx.Commit()
		}
	}()

	err = fn(tx)
	return err
}

func Confirm (message string, fn func()) {
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
		Label: message,
		Validate: validate,
	}

	result, err := prompt.Run()
	if err != nil {
		panic(err)
	}

	if (result == "N" || result == "n") {
		// Stop execute!
		return
	}

	fn()
}