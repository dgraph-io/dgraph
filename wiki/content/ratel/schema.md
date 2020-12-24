+++
date = "2020-31-08T19:35:35+11:00"
title = "Schema"
[menu.main]
	identifier = "schema-management"
    parent = "ratel"
    weight = 3
+++

## Predicate Section

You have two panels: 
- The left panel with a predicate list in a table. The table consists of three columns, the name of the `predicate`, the `type`, and the `indices`. 
- On the right panel you have the properties related to the selection from the right panel.

![Ratel Schema](/images/ratel/ratel_schema.png)

You can add new predicates using the `Add Predicate` button on the top left corner. In the dialog box, you can add the name of the predicate, the type, language if required, and its indices.

In the predicate's `Properties` panel you can edit the type, the indexation, or drop it. In the tab `Samples & Statistics` you have information about your dataset. It has a sample sneak-peek of the predicate's data.

## Type Definition Section

You have two panels:
- The left panel provides you a table with a type list, with two columns: `Type` and `Field Count`. 
- On the right panel you have the properties related to the selected `Type`.

![Ratel Schema](/images/ratel/ratel_schema_types.png)

 You can add new Types using the `Add Type` button on the top left corner. In the dialog box, you can add the name of the Type and select which predicates belong to this Type. The list will show only existing predicates.

## Bulk Edit Schema & Drop Data

With this option you can edit the schema directly in plain-text. You also have the option to Drop the data. 

{{% notice "note" %}}
There are two ways to drop the DB. The default option will drop the data but keep the `Schema`. To drop everything you have to select the check-box `Also drop Schema and Types`.
{{% /notice %}}

![Ratel Schema](/images/ratel/ratel_schema_bulk.png)
