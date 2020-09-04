+++
date = "2020-31-08T19:35:35+11:00"
title = "Schema"
[menu.main]
    parent = "ratel"
    weight = 3
+++

## Schema Management

## Bulk Edit Schema & Drop Data

With this option, you can edit the schema directly in plaintext. You also have the option to Drop the data. There are two ways to drop the DB. The default option will drop the data but keep the Schema. To drop everything you can select check-box "Also drop Schema and Types".

### Predicate Section

You have two panels, the right panel with the list of predicates in a table. The table consists of three columns, the name of the predicate, the type, and the indices. The left panel you have the properties related to the selection you have don in the right panel.

The button on the right top called "Add Predicate", you can add new predicates. In the dialog box, you can add the name of the predicate, the type, language if so, and its indices.

The properties panel for predicate you can edit the type, the indexation, or drop it. In the tab "samples & Statistics" you have information about your dataset. It has a sneak pick of the data.

### Type Definition Section

You have two panels, the right panel with the list of types in a table. With two columns Type and Field Count. The left panel you have the properties related to the Type.

The button on the right top called "Add Type", you can add new Types. In the dialog box, you can add the name of the Type and select what predicates belong to this Type. The list will show only existing predicates.

