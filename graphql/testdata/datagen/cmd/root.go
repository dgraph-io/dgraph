/*
Copyright © 2020 NAME HERE <EMAIL ADDRESS>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	graphqlServerUrl              = "graphqlServerUrl"
	authorizationHeader           = "authorizationHeader"
	jwt                           = "jwt"
	restaurantDataFilePath        = "restaurantDataFilePath"
	maxErrorsInRestaurantAddition = "maxErrorsInRestaurantAddition"
	maxDishes4AnyRestaurant       = "maxDishes4AnyRestaurant"
	httpTimeout                   = "httpTimeout"
)

var cfgFile string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "datagen",
	Short: "Data Generator for Cool GraphQL App: Restaurant Delivery",
	Long: `datagen is a Data Generator. Currently, it generates only Restaurant & Dish data 
for the Cool GraphQL App. It uses a specific data file to create Restaurants, 
while it generates Dishes on the fly by itself. Later, it may support generating 
both Restaurant and Dish data on the fly using some seed data.`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	Run: func(cmd *cobra.Command, args []string) {
		run()
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "",
		"config file (default is $HOME/.config/datagen/conf.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().StringP(graphqlServerUrl, "u", "", "Url of the GraphQL server")
	rootCmd.Flags().StringP(authorizationHeader, "a", "", "Key for auth header")
	rootCmd.Flags().StringP(jwt, "j", "", "JWT to be sent in auth header")
	rootCmd.Flags().StringP(restaurantDataFilePath, "f", "", "Path to restaurant data file")
	rootCmd.Flags().StringP(maxErrorsInRestaurantAddition, "e", "",
		"Maximum number of errors to ignore during Restaurant addition, before exiting")
	rootCmd.Flags().StringP(maxDishes4AnyRestaurant, "d", "",
		"Maximum number of dishes any Restaurant can have")
	rootCmd.Flags().StringP(httpTimeout, "t", "", "Timeout for http requests")

	// add bindings with viper
	_ = viper.BindPFlag(graphqlServerUrl, rootCmd.Flags().Lookup(graphqlServerUrl))
	_ = viper.BindPFlag(authorizationHeader, rootCmd.Flags().Lookup(authorizationHeader))
	_ = viper.BindPFlag(jwt, rootCmd.Flags().Lookup(jwt))
	_ = viper.BindPFlag(restaurantDataFilePath, rootCmd.Flags().Lookup(restaurantDataFilePath))
	_ = viper.BindPFlag(maxErrorsInRestaurantAddition, rootCmd.Flags().Lookup(maxErrorsInRestaurantAddition))
	_ = viper.BindPFlag(maxDishes4AnyRestaurant, rootCmd.Flags().Lookup(maxDishes4AnyRestaurant))
	_ = viper.BindPFlag(httpTimeout, rootCmd.Flags().Lookup(httpTimeout))
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// Search config in home directory with name ".datagen" (without extension).
		viper.AddConfigPath(filepath.Join(home, ".config", "datagen"))
		viper.SetConfigName("conf")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}

	// make sure we have everything required to generate data
	checkConfigSanity()
}

func run() {
	readStartTs := time.Now()
	data, err := readRestaurantData()
	if err != nil {
		log.Fatal(err)
	}
	readStopTS := time.Now()

	// add countries initially
	addCountries()
	finishCountriesTs := time.Now()

	// now add restaurants
	invalid, added, failed := createAndAddRestaurants(data)
	finishRestaurantsTS := time.Now()
	timeInRestaurant := finishRestaurantsTS.Sub(finishCountriesTs)

	log.Println()
	log.Println("==============")
	log.Println("::Statistics::")
	log.Println("==============")
	log.Println("Total time taken: ", finishRestaurantsTS.Sub(readStartTs).String())
	log.Println("Time taken for reading data: ", readStopTS.Sub(readStartTs).String())
	log.Println("Time taken for adding countries: ", finishCountriesTs.Sub(readStopTS).String())
	log.Println("Time taken for adding restaurants: ", timeInRestaurant.String())
	log.Println("Avg. Time taken per restaurant: ",
		time.Duration(int(timeInRestaurant)/(invalid+added+failed)).String())
	log.Println("Avg. Time taken per added restaurant: ",
		time.Duration(int(timeInRestaurant)/added).String())
}

func checkConfigSanity() {
	if strings.TrimSpace(viper.GetString(graphqlServerUrl)) == "" {
		log.Fatal("graphqlServerUrl is required")
	}
	if _, err := url.Parse(viper.GetString(graphqlServerUrl)); err != nil {
		log.Fatal("invalid graphqlServerUrl", err)
	}
	if strings.TrimSpace(viper.GetString(authorizationHeader)) == "" {
		log.Fatal("authorizationHeader is required")
	}
	if strings.TrimSpace(viper.GetString(jwt)) == "" {
		log.Fatal("jwt is required")
	}
	// data file will get checked for errors when its being read
	if viper.GetInt(maxErrorsInRestaurantAddition) < 0 {
		log.Fatal("maxErrorsInRestaurantAddition must be >= 0")
	}
	if viper.GetInt(maxDishes4AnyRestaurant) <= 0 || viper.GetInt(maxDishes4AnyRestaurant) > 5000 {
		log.Fatal("maxDishes4AnyRestaurant must be in the range (0,5000]")
	}
}
