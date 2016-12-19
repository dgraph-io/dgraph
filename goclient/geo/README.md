README
======

You can find sample files to upload to dgraph at:
https://github.com/dgraph-io/benchmarks/tree/master/geodata

## List of sample files
- **countries.geojson**: Country polygons from Natural Earth.
- **states_provinces.geojson**: State/province polygons from Natural Earth
- **intl_airports.geojson**: International aiport locations from Natural Earth
- **us_airports.geojson**: US airports locations from GeoCommons.
- **zcta5.json**: US Zip code polygons. Note: This file is very large.

## Uploading files to dgraph

To upload files geojson to dgraph you can use the client as follows:

```
# Countries: Use ADM0_A3 as the unique key for countries.
$ dgraphgoclient --json countries.geojson --geoid ADM0_A3

# States/Provinces
$ dgraphgoclient --json states_provinces.geojson --geoid OBJECTID_1

# Intl airports
$ dgraphgoclient --json intl_airports.geojson --geoid abbrev

# US airports
$ dgraphgoclient --json us_airports.geojson

# Zip codes. Note this command will take a while to finish.
$ dgraphgoclient --json zcta5.json --geoid GEOID10

```


