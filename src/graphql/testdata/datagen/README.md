# README

### About
This is `datagen`. A command line tool to generate data using Dgraph's GraphQL API. At present
, it is written for a [specific schema](schema.graphql), and so generates data only for that
 schema. It uses an existing [dataset](data/zomato-restaurants-data.zip) which contains data
  about Restaurants, while it generates Dish data at random.

### Usage
It needs a running dgraph instance to work. So, let's start a dgraph instance first. Follow these
 steps:
* `$ mkdir ~/__data && cd ~/__data` - we will start dgraph zero and alpha in this directory, so
 that the data is stored here, and can be reused later whenever required.
* `$ dgraph zero`
* `$ dgraph alpha`

Now, change your working directory to the directory containing this README file, and run
 following commands:
1. `$ go build`
2. `$ curl -X POST localhost:8080/admin/schema --data-binary '@schema.graphql'`
3. `$ unzip data/zomato-restaurants-data.zip -d data`
4. The above command will output some JSON files in the data directory. Out of them `file1.json
` is corrupt, rest will work.
5. Edit the `conf.yaml`:
    * set `restaurantDataFilePath` to `data/file2.json` - i.e., we are importing the data in `file2
  .json` to dgraph.
    * set `maxErrorsInRestaurantAddition` to `1000000`. Basically, a very high value, as some of
   the restaurants are duplicates in and across the data files.
    * set `maxDishes4AnyRestaurant` to `1000` - i.e., every restaurant will have at max 1000 Dishes.
    * `authorizationHeader` and `jwt` in configuration refer to the header and JWT values for
     `@auth` directive, if you have any in your schema. In the schema given with this, there is
      no `@auth` directive, so no need to pay any attention to them.
6. `$ ./datagen --config conf.yaml` - this will start the data generator using the configuration
 file. Once it finishes, all the data in `data/file2.json` would have been imported into dgraph.
7. Repeat steps 5 & 6 with different data files. i.e., keep setting `restaurantDataFilePath` to
 other data files and importing them.
8. This is all that is required to import the data in dgraph. Now you can stop alpha and zero
, and keep the `~/__data` directory safe to reuse it later.
 
 You can always look for help with `$ ./datagen --help`