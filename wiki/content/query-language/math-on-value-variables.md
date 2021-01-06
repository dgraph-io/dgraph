+++
date = "2017-03-20T22:25:17+11:00"
title = "Math on value variables"
weight = 12
[menu.main]
    parent = "query-language"
+++

Value variables can be combined using mathematical functions.  For example, this could be used to associate a score which is then used to order or perform other operations, such as might be used in building news feeds, simple recommendation systems, and so on.

Math statements must be enclosed within `math( <exp> )` and must be stored to a value variable.

The supported operators are as follows:

| Operators                       | Types accepted                                 | What it does                                                   |
| :------------:                  | :--------------:                               | :------------------------:                                     |
| `+` `-` `*` `/` `%`             | `int`, `float`                                     | performs the corresponding operation                           |
| `min` `max`                     | All types except `geo`, `bool`  (binary functions) | selects the min/max value among the two                        |
| `<` `>` `<=` `>=` `==` `!=`     | All types except `geo`, `bool`                     | Returns true or false based on the values                      |
| `floor` `ceil` `ln` `exp` `sqrt` | `int`, `float` (unary function)                    | performs the corresponding operation                           |
| `since`                         | `dateTime`                                 | Returns the number of seconds in float from the time specified |
| `pow(a, b)`                     | `int`, `float`                                     | Returns `a to the power b`                                     |
| `logbase(a,b)`                  | `int`, `float`                                     | Returns `log(a)` to the base `b`                               |
| `cond(a, b, c)`                 | first operand must be a boolean                | selects `b` if `a` is true else `c`                            |


Query Example:  Form a score for each of Steven Spielberg's movies as the sum of number of actors, number of genres and number of countries.  List the top five such movies in order of decreasing score.

{{< runnable >}}
{
	var(func:allofterms(name@en, "steven spielberg")) {
		films as director.film {
			p as count(starring)
			q as count(genre)
			r as count(country)
			score as math(p + q + r)
		}
	}

	TopMovies(func: uid(films), orderdesc: val(score), first: 5){
		name@en
		val(score)
	}
}
{{< /runnable >}}

Value variables and aggregations of them can be used in filters.

Query Example: Calculate a score for each Steven Spielberg movie with a condition on release date to penalize movies that are more than 10 years old, filtering on the resulting score.

{{< runnable >}}
{
  var(func:allofterms(name@en, "steven spielberg")) {
    films as director.film {
      p as count(starring)
      q as count(genre)
      date as initial_release_date
      years as math(since(date)/(365*24*60*60))
      score as math(cond(years > 10, 0, ln(p)+q-ln(years)))
    }
  }

  TopMovies(func: uid(films), orderdesc: val(score)) @filter(gt(val(score), 2)){
    name@en
    val(score)
    val(date)
  }
}
{{< /runnable >}}


Values calculated with math operations are stored to value variables and so can be aggregated.

Query Example: Compute a score for each Steven Spielberg movie and then aggregate the score.

{{< runnable >}}
{
	steven as var(func:eq(name@en, "Steven Spielberg")) @filter(has(director.film)) {
		director.film {
			p as count(starring)
			q as count(genre)
			r as count(country)
			score as math(p + q + r)
		}
		directorScore as sum(val(score))
	}

	score(func: uid(steven)){
		name@en
		val(directorScore)
	}
}
{{< /runnable >}}
