/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"context"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/testutil"
	"github.com/stretchr/testify/require"
)

var tc = []struct {
	query string
	resp  string
}{
	{
		query: `{
		  caro(func: allofterms(name@en, "Marc Caro")) {
			  name@en
			  director.film {
			    name@en
			  }
		  }
		}`,
		resp: `{
			"caro": [
				{
					"name@en": "Marc Caro",
					"director.film": [
						{
							"name@en": "Delicatessen"
						},
						{
							"name@en": "The City of Lost Children"
						}
					]
				}
			]
		}`,
	},
	{
		query: `{
			coactors(func:allofterms(name@en, "Jane Campion")) @cascade {
				JC_films as director.film {      # JC_films = all Jane Campion's films
					starting_movie: name@en
					starring {
						JC_actors as performance.actor {      # JC_actors = all actors in all JC films
							actor : name@en
							actor.film {
								performance.film @filter(not uid(JC_films)) {
									film_together : name@en
									starring {
										# find a coactor who has been in some JC film
										performance.actor @filter(uid(JC_actors)) {
											coactor_name: name@en
										}
									}
								}
							}
						}
					}
				}
			}
		}`,
		resp: `{
			"coactors": [
				{
					"director.film": [
						{
							"starting_movie": "Bright Star",
							"starring": [
								{
									"performance.actor": [
										{
											"actor": "Ben Whishaw",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "Cloud Atlas",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ben Whishaw"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ben Whishaw"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ben Whishaw"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ben Whishaw"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ben Whishaw"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "I'm Not There",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Christian Bale"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ben Whishaw"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Christian Bale"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Cloud Atlas",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ben Whishaw"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ben Whishaw"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ben Whishaw"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ben Whishaw"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ben Whishaw"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Perfume: The Story of a Murderer",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ben Whishaw"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Skyfall",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ben Whishaw"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The International",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ben Whishaw"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Cloud Atlas",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ben Whishaw"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ben Whishaw"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ben Whishaw"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ben Whishaw"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ben Whishaw"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Cloud Atlas",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ben Whishaw"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ben Whishaw"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ben Whishaw"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ben Whishaw"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ben Whishaw"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Layer Cake",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ben Whishaw"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Cloud Atlas",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ben Whishaw"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ben Whishaw"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ben Whishaw"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ben Whishaw"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ben Whishaw"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Zero Theorem",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ben Whishaw"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "In the Heart of the Sea",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ben Whishaw"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Abbie Cornish",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "A Good Year",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Abbie Cornish"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Sucker Punch",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Abbie Cornish"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Elizabeth: The Golden Age",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Abbie Cornish"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Horseplay",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Abbie Cornish"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Marking Time",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Abbie Cornish"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Everything Goes",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Abbie Cornish"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Legend of the Guardians: The Owls of Ga'Hoole",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Sam Neill"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Abbie Cornish"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Thomas Brodie-Sangster",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "Station Jim",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Thomas Brodie-Sangster"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Love Actually",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Thomas Brodie-Sangster"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Miracle of the Cards",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Thomas Brodie-Sangster"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Hitler: The Rise of Evil",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Thomas Brodie-Sangster"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Jonathan Aris",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "Not Only But Always",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Jonathan Aris"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Sightseers",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Jonathan Aris"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Jackal",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Jonathan Aris"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Claudie Blakley",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "Pride & Prejudice",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Claudie Blakley"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Roger Ashton-Griffiths",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "You Will Meet a Tall Dark Stranger",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Roger Ashton-Griffiths"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Madness of King George",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Roger Ashton-Griffiths"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Pirates",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Roger Ashton-Griffiths"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Swept from the Sea",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Roger Ashton-Griffiths"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Napoleon",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Roger Ashton-Griffiths"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Grace of Monaco",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Roger Ashton-Griffiths"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Nicole Kidman"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Brothers Grimm",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Roger Ashton-Griffiths"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Cook, the Thief, His Wife & Her Lover",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Roger Ashton-Griffiths"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Red Faction: Origins",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Roger Ashton-Griffiths"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Gangs of New York",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Roger Ashton-Griffiths"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Young Sherlock Holmes",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Roger Ashton-Griffiths"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Vincent Franklin",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "Confetti",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Vincent Franklin"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Paul Schneider",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "Away We Go",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Paul Schneider"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Goodbye to All That",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Paul Schneider"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Elizabethtown",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Paul Schneider"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Olly Alexander",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "Great Expectations",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Olly Alexander"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Kerry Fox",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "Mister Pip",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kerry Fox"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Burning Man",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kerry Fox"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Last Tattoo",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kerry Fox"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Samuel Barnett",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "Mrs Henderson Presents",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Samuel Barnett"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The History Boys",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Samuel Barnett"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								}
							]
						},
						{
							"starting_movie": "In the Cut",
							"starring": [
								{
									"performance.actor": [
										{
											"actor": "Arthur J. Nascarella",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "Kate & Leopold",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Arthur J. Nascarella"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Meg Ryan"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Groomsmen",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Arthur J. Nascarella"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Bringing Out the Dead",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Theo Kogan"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Arthur J. Nascarella"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Cliff Curtis"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Cop Land",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Arthur J. Nascarella"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Running Scared",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Arthur J. Nascarella"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "New Jersey Drive",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Arthur J. Nascarella"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Ref",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Arthur J. Nascarella"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Mark Ruffalo",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "My Life Without Me",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Mark Ruffalo"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Eternal Sunshine of the Spotless Mind",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Mark Ruffalo"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kate Winslet"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Dentist",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Mark Ruffalo"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Collateral",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Mark Ruffalo"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Date Night",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Mark Ruffalo"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Now You See Me",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Mark Ruffalo"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Sympathy for Delicious",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Mark Ruffalo"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Shutter Island",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Mark Ruffalo"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Zodiac",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Mark Ruffalo"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "All the King's Men",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kate Winslet"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Mark Ruffalo"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Ride with the Devil",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Mark Ruffalo"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Rumor Has It",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Mark Ruffalo"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Julius LeFlore",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "Police Academy: Mission to Moscow",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Julius LeFlore"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Michael Ienna",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "Little Fish",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Sam Neill"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Michael Ienna"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Vinny Vella",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "Casino",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Vinny Vella"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Coffee and Cigarettes",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Vinny Vella"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Lou Martini Jr.",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "The Godfather",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Lou Martini Jr."
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Michelle DiBenedetti",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "Hitch",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Michelle DiBenedetti"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Theo Kogan",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "Bringing Out the Dead",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Theo Kogan"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Arthur J. Nascarella"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Cliff Curtis"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Kevin Bacon",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "My One and Only",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kevin Bacon"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Apollo 13",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kevin Bacon"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Sleepers",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kevin Bacon"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "National Lampoon's Animal House",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kevin Bacon"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Taking Chance",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kevin Bacon"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Tremors",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kevin Bacon"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Hollow Man",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kevin Bacon"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Footloose",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kevin Bacon"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "A Few Good Men",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kevin Bacon"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Queens Logic",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kevin Bacon"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "X-Men: First Class",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kevin Bacon"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Frost/Nixon",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kevin Bacon"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Murder in the First",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kevin Bacon"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Diner",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kevin Bacon"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Mystic River",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kevin Bacon"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Flatliners",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kevin Bacon"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Beyond All Boundaries",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kevin Bacon"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Dana Lubotsky",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "Mona Lisa Smile",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Dana Lubotsky"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Babe",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Dana Lubotsky"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Jennifer Jason Leigh",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "Girls of the White Orchid",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Jennifer Jason Leigh"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Love Letter",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Jennifer Jason Leigh"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Skipped Parts",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Jennifer Jason Leigh"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Hudsucker Proxy",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Jennifer Jason Leigh"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Flesh and Blood",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Jennifer Jason Leigh"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Sister, Sister",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Jennifer Jason Leigh"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Backdraft",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Jennifer Jason Leigh"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Man Who Wasn't There",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Jennifer Jason Leigh"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Road to Perdition",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Jennifer Jason Leigh"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Last Exit to Brooklyn",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Jennifer Jason Leigh"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Fast Times at Ridgemont High",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Jennifer Jason Leigh"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Georgia",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Jennifer Jason Leigh"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Thanks of a Grateful Nation",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Jennifer Jason Leigh"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "A Thousand Acres",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Jennifer Jason Leigh"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Meg Ryan",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "When Harry Met Sally...",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Meg Ryan"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Innerspace",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Meg Ryan"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Armed and Dangerous",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Meg Ryan"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Promised Land",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Meg Ryan"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "City of Angels",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Meg Ryan"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Top Gun",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Meg Ryan"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Restoration",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Sam Neill"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Meg Ryan"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "I.Q.",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Meg Ryan"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Deal",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Meg Ryan"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Kate & Leopold",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Arthur J. Nascarella"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Meg Ryan"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Michelle Hurst",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "All Good Things",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Michelle Hurst"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Smoke",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Michelle Hurst"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								}
							]
						},
						{
							"starting_movie": "Holy Smoke!",
							"starring": [
								{
									"performance.actor": [
										{
											"actor": "Tim Robertson",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "Jenny Kissed Me",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Tim Robertson"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Chant of Jimmie Blacksmith",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Tim Robertson"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Julie Hamilton",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "In the Company of Actors",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Justine Clarke"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Julie Hamilton"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Kerry Walker",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "The Singer and the Dancer",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kerry Walker"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Paul Goddard",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "Babe",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Paul Goddard"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Hildegarde",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Paul Goddard"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Richard E. Grant"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Matrix",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Paul Goddard"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Leslie Dayman",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "Oscar and Lucinda",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Chris Haywood"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Leslie Dayman"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Pam Grier",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "First to Die",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Pam Grier"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Fortress 2: Re-Entry",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Pam Grier"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Adventures of Pluto Nash",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Pam Grier"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Class of 1999",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Pam Grier"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Larry Crowne",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Pam Grier"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Jackie Brown",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Pam Grier"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Package",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Pam Grier"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Arena",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Pam Grier"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Mars Attacks!",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Pam Grier"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Greased Lightning",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Pam Grier"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Above the Law",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Pam Grier"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Drum",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Pam Grier"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Harvey Keitel",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "Wrong Turn at Tahoe",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "From Dusk till Dawn",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Bad Timing",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Blue Collar",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "National Treasure: Book of Secrets",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Taking Sides",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Duellists",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Grey Zone",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Red Dragon",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Mary-Louise Parker"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Inglourious Basterds",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Moonrise Kingdom",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Taxi Driver",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Cop Land",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Arthur J. Nascarella"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Alice Doesn't Live Here Anymore",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Blue in the Face",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Smoke",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Michelle Hurst"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Who's That Knocking at My Door",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "National Treasure",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Grand Budapest Hotel",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Pulp Fiction",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Mean Streets",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Thelma & Louise",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Last Temptation of Christ",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Barbara Hershey"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Reservoir Dogs",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Bugsy",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Bad Lieutenant",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Kate Winslet",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "War Game",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kate Winslet"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kate Winslet"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Life of David Gale",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kate Winslet"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Holiday",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kate Winslet"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Movie 43",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kate Winslet"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Iris",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kate Winslet"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Enigma",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kate Winslet"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "All the King's Men",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kate Winslet"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Mark Ruffalo"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Pride",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kate Winslet"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Flushed Away",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kate Winslet"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Reader",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kate Winslet"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Christmas Carol: The Movie",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kate Winslet"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "War Game",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kate Winslet"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kate Winslet"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Carnage",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kate Winslet"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Titanic",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kate Winslet"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Heavenly Creatures",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kate Winslet"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Sense and Sensibility",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kate Winslet"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "A Kid in King Arthur's Court",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kate Winslet"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Contagion",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kate Winslet"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Revolutionary Road",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kate Winslet"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Eternal Sunshine of the Spotless Mind",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Mark Ruffalo"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kate Winslet"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Little Children",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kate Winslet"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Quills",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kate Winslet"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Hamlet",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "John Gielgud"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kate Winslet"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Labor Day",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kate Winslet"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Nalini Krishan",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "Star Wars Episode II: Attack of the Clones",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Nalini Krishan"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								}
							]
						},
						{
							"starting_movie": "The Water Diary",
							"starring": [
								{
									"performance.actor": [
										{
											"actor": "Alice Englert",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "Beautiful Creatures",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Alice Englert"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Ginger and Rosa",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Alice Englert"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Justine Clarke",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "In the Company of Actors",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Justine Clarke"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Julie Hamilton"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Mad Max Beyond Thunderdome",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Justine Clarke"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Russell Dykstra",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "Garage Days",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Russell Dykstra"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Oranges and Sunshine",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Russell Dykstra"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Clubland",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Russell Dykstra"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Lantana",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Barbara Hershey"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Russell Dykstra"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Ned Kelly",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Clayton Jacobson"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Russell Dykstra"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Clayton Jacobson",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "Ned Kelly",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Clayton Jacobson"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Russell Dykstra"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Chris Haywood",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "The Nun and the Bandit",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Chris Haywood"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Salvation",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Chris Haywood"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Lonely Hearts",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Chris Haywood"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Oscar and Lucinda",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Chris Haywood"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Leslie Dayman"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Exile",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Chris Haywood"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Emerald City",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Nicole Kidman"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Chris Haywood"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Island",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Chris Haywood"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Man from Snowy River",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Chris Haywood"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Sweet Talker",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Chris Haywood"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Burke & Wills",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Chris Haywood"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Jindabyne",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Chris Haywood"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Death Cheaters",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Chris Haywood"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Molokai: The Story of Father Damien",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Sam Neill"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Chris Haywood"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Alex",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Chris Haywood"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Lust and Revenge",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Chris Haywood"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Bit Part",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Chris Haywood"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Nicole Kidman"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								}
							]
						},
						{
							"starting_movie": "The Portrait of a Lady",
							"starring": [
								{
									"performance.actor": [
										{
											"actor": "Shelley Winters",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "The Tenant",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Shelley Winters"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Odds Against Tomorrow",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Shelley Winters"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Poor Pretty Eddie",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Shelley Winters"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Rudolph and Frosty's Christmas in July",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Shelley Winters"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Silence of the Hams",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Shelley Winters"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Jury Duty",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Shelley Winters"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Lolita",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Shelley Winters"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Mary-Louise Parker",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "Mr. Wonderful",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Mary-Louise Parker"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Red Dragon",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Mary-Louise Parker"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Fried Green Tomatoes",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Mary-Louise Parker"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Bullets over Broadway",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Mary-Louise Parker"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Client",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Mary-Louise Parker"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Boys on the Side",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Mary-Louise Parker"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Red 2",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "John Malkovich"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Mary-Louise Parker"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Christian Bale",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "Mary, Mother of Jesus",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Christian Bale"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Out of the Furnace",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Christian Bale"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Henry V",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Christian Bale"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "I'm Not There",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Christian Bale"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ben Whishaw"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Christian Bale"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Mio in the Land of Faraway",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Christian Bale"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Treasure Island",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Christian Bale"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Public Enemies",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Christian Bale"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Velvet Goldmine",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Christian Bale"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Little Women",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Christian Bale"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Newsies",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Christian Bale"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "A Midsummer Night's Dream",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Christian Bale"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "3:10 to Yuma",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Christian Bale"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Empire of the Sun",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "John Malkovich"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Christian Bale"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "I'm Not There",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Christian Bale"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ben Whishaw"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Christian Bale"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The New World",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Christian Bale"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Knight of Cups",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Christian Bale"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "John Gielgud",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "Leave All Fair",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "John Gielgud"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Scarlet and the Black",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "John Gielgud"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Prospero's Books",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "John Gielgud"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Hamlet",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "John Gielgud"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kate Winslet"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Murder by Decree",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "John Gielgud"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Elizabeth",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "John Gielgud"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Chimes at Midnight",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "John Gielgud"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Priest of Love",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "John Gielgud"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Richard III",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "John Gielgud"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Julius Caesar",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "John Gielgud"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Martin Donovan",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "Heaven",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Martin Donovan"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Unthinkable",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Martin Donovan"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Living Out Loud",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Holly Hunter"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Martin Donovan"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Reluctant Fundamentalist",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Martin Donovan"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Nicole Kidman",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "Trespass",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Nicole Kidman"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Hemingway & Gellhorn",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Nicole Kidman"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "BMX Bandits",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Nicole Kidman"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Malice",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Nicole Kidman"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Cold Mountain",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Nicole Kidman"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Railway Man",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Nicole Kidman"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Invasion",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Nicole Kidman"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Emerald City",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Nicole Kidman"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Chris Haywood"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Far and Away",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Nicole Kidman"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Golden Compass",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Nicole Kidman"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Just Go With It",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Nicole Kidman"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Batman Forever",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Nicole Kidman"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Grace of Monaco",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Roger Ashton-Griffiths"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Nicole Kidman"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Hours",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Nicole Kidman"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "To Die For",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Nicole Kidman"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Eyes Wide Shut",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Nicole Kidman"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Happy Feet",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Nicole Kidman"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Days of Thunder",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Nicole Kidman"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Peacemaker",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Nicole Kidman"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Bit Part",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Chris Haywood"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Nicole Kidman"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Valentina Cervi",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "The Tulse Luper Suitcases",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Valentina Cervi"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Hotel",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Valentina Cervi"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "John Malkovich"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Shelley Duvall",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "Annie Hall",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Shelley Duvall"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "RocketMan",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Shelley Duvall"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Suburban Commando",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Shelley Duvall"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Frankenweenie (1984)",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Shelley Duvall"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Time Bandits",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Shelley Duvall"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Home Fries",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Shelley Duvall"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Roxanne",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Shelley Duvall"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Underneath",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Shelley Duvall"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Shining Forwards and Backwards",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Shelley Duvall"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Shining",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Shelley Duvall"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Tale of the Mummy",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Shelley Duvall"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Roger Ashton-Griffiths",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "You Will Meet a Tall Dark Stranger",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Roger Ashton-Griffiths"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Madness of King George",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Roger Ashton-Griffiths"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Pirates",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Roger Ashton-Griffiths"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Swept from the Sea",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Roger Ashton-Griffiths"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Napoleon",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Roger Ashton-Griffiths"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Grace of Monaco",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Roger Ashton-Griffiths"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Nicole Kidman"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Brothers Grimm",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Roger Ashton-Griffiths"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Cook, the Thief, His Wife & Her Lover",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Roger Ashton-Griffiths"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Red Faction: Origins",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Roger Ashton-Griffiths"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Gangs of New York",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Roger Ashton-Griffiths"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Young Sherlock Holmes",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Roger Ashton-Griffiths"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "John Malkovich",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "In the Line of Fire",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "John Malkovich"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Hitchhiker's Guide to the Galaxy",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "John Malkovich"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Beowulf",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "John Malkovich"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Empire of the Sun",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "John Malkovich"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Christian Bale"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Transformers: Dark of the Moon",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "John Malkovich"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Heart of Darkness",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "John Malkovich"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Afterwards",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "John Malkovich"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Mary Reilly",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "John Malkovich"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "John Malkovich"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Red 2",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "John Malkovich"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Mary-Louise Parker"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Burn After Reading",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "John Malkovich"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Changeling",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "John Malkovich"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Penguins of Madagascar",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "John Malkovich"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Mary Reilly",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "John Malkovich"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "John Malkovich"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Hotel",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Valentina Cervi"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "John Malkovich"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Dangerous Liaisons",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "John Malkovich"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Richard E. Grant",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "Doctor Who: The Curse of Fatal Death",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Richard E. Grant"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Penelope",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Richard E. Grant"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "L.A. Story",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Richard E. Grant"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Foster",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Richard E. Grant"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Twelfth Night: Or What You Will",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Richard E. Grant"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Warlock",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Richard E. Grant"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Hildegarde",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Paul Goddard"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Richard E. Grant"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Nutcracker in 3D",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Richard E. Grant"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Age of Innocence",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Richard E. Grant"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Bram Stoker's Dracula",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Richard E. Grant"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Little Vampire",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Richard E. Grant"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Corpse Bride",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Richard E. Grant"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Viggo Mortensen",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "Ruby Cairo",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Viggo Mortensen"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "A Perfect Murder",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Viggo Mortensen"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Hidalgo",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Viggo Mortensen"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Lord of the Rings: The Return of the King",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Viggo Mortensen"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Crimson Tide",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Viggo Mortensen"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Young Americans",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Viggo Mortensen"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Road",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Viggo Mortensen"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Young Guns II",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Viggo Mortensen"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "On the Road",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Viggo Mortensen"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Psycho",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Viggo Mortensen"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Lord of the Rings: The Two Towers",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Bruce Allpress"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Viggo Mortensen"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "G.I. Jane",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Viggo Mortensen"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Lord of the Rings: The Fellowship of the Ring",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ian Mune"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Viggo Mortensen"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Carlito's Way",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Viggo Mortensen"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Leatherface: The Texas Chainsaw Massacre III",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Viggo Mortensen"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Last Voyage of Demeter",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Viggo Mortensen"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Barbara Hershey",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "The Natural",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Barbara Hershey"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Tin Men",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Barbara Hershey"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Falling Down",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Barbara Hershey"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Lantana",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Barbara Hershey"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Russell Dykstra"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Beaches",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Barbara Hershey"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Right Stuff",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Barbara Hershey"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Entity",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Barbara Hershey"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Baby Maker",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Barbara Hershey"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Last Temptation of Christ",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Barbara Hershey"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Hannah and Her Sisters",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Barbara Hershey"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								}
							]
						},
						{
							"starting_movie": "The Piano",
							"starring": [
								{
									"performance.actor": [
										{
											"actor": "Ian Mune",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "The Lord of the Rings: The Fellowship of the Ring",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ian Mune"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Viggo Mortensen"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Spooked",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Cliff Curtis"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ian Mune"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Two Little Boys",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ian Mune"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Nutcase",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ian Mune"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Harvey Keitel",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "Wrong Turn at Tahoe",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "From Dusk till Dawn",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Bad Timing",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Blue Collar",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "National Treasure: Book of Secrets",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Taking Sides",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Duellists",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Grey Zone",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Red Dragon",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Mary-Louise Parker"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Inglourious Basterds",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Moonrise Kingdom",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Taxi Driver",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Cop Land",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Arthur J. Nascarella"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Alice Doesn't Live Here Anymore",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Blue in the Face",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Smoke",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Michelle Hurst"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Who's That Knocking at My Door",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "National Treasure",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Grand Budapest Hotel",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Pulp Fiction",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Mean Streets",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Thelma & Louise",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Last Temptation of Christ",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Barbara Hershey"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Reservoir Dogs",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Bugsy",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Bad Lieutenant",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Harvey Keitel"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Sam Neill",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "Jurassic Park",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Sam Neill"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "One Against the Wind",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Sam Neill"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "A Long Way Down",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Sam Neill"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Sleeping Dogs",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Sam Neill"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Adventurer: The Curse of the Midas Box",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Sam Neill"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Merlin",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Sam Neill"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "For Love Alone",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Sam Neill"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Wimbledon",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Sam Neill"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Restoration",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Sam Neill"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Meg Ryan"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Evil Angels",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Sam Neill"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "My Brilliant Career",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Sam Neill"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Framed",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Sam Neill"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Magic Pudding",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Sam Neill"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Under the Mountain",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Sam Neill"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Little Fish",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Sam Neill"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Michael Ienna"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "La Révolution française",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Sam Neill"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Plenty",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Sam Neill"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Jurassic Park III",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Sam Neill"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Fever",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Sam Neill"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Molokai: The Story of Father Damien",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Sam Neill"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Chris Haywood"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Jungle Book",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Sam Neill"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Event Horizon",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Sam Neill"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Legend of the Guardians: The Owls of Ga'Hoole",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Sam Neill"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Abbie Cornish"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Hunt for Red October",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Sam Neill"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Yes",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Sam Neill"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Kerry Walker",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "The Singer and the Dancer",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Kerry Walker"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Anna Paquin",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "The Courageous Heart of Irena Sendler",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Anna Paquin"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "X-Men: Days of Future Past",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Anna Paquin"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Buffalo Soldiers",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Anna Paquin"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "X-Men 2",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Anna Paquin"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Anna Paquin"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Amistad",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Anna Paquin"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "X-Men",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Anna Paquin"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Almost Famous",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Anna Paquin"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "X-Men 2",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Anna Paquin"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Anna Paquin"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Romantics",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Anna Paquin"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Finding Forrester",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Anna Paquin"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Scream 4",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Anna Paquin"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "X-Men: The Last Stand",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Anna Paquin"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Holly Hunter",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "Harlan County War",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Holly Hunter"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Broadcast News",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Holly Hunter"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Positively True Adventures of the Alleged Texas Cheerleader-Murdering Mom",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Holly Hunter"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Big White",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Holly Hunter"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Always",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Holly Hunter"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "O Brother, Where Art Thou?",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Holly Hunter"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Timecode",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Holly Hunter"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Once Around",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Holly Hunter"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Blood Simple",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Holly Hunter"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Living Out Loud",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Holly Hunter"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Martin Donovan"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Raising Arizona",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Holly Hunter"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Moonlight Mile",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Holly Hunter"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Copycat",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Holly Hunter"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Cliff Curtis",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "Blow",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Cliff Curtis"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Six Days Seven Nights",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Cliff Curtis"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Spooked",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Cliff Curtis"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Ian Mune"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Deep Rising",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Cliff Curtis"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Whale Rider",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Cliff Curtis"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Bringing Out the Dead",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Theo Kogan"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Arthur J. Nascarella"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Cliff Curtis"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Collateral Damage",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Cliff Curtis"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Crossing Over",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Cliff Curtis"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								},
								{
									"performance.actor": [
										{
											"actor": "Bruce Allpress",
											"actor.film": [
												{
													"performance.film": [
														{
															"film_together": "The Scarecrow",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Bruce Allpress"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "Ozzie",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Bruce Allpress"
																		}
																	]
																}
															]
														}
													]
												},
												{
													"performance.film": [
														{
															"film_together": "The Lord of the Rings: The Two Towers",
															"starring": [
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Bruce Allpress"
																		}
																	]
																},
																{
																	"performance.actor": [
																		{
																			"coactor_name": "Viggo Mortensen"
																		}
																	]
																}
															]
														}
													]
												}
											]
										}
									]
								}
							]
						}
					]
				}
			]
		}`,
	},
	{
		query: `{
			PJ as var(func:allofterms(name@en, "Peter Jackson")) @normalize @cascade {
				F as director.film
			}

			peterJ(func: uid(PJ)) @normalize @cascade {
				name : name@en
				actor.film {
					performance.film @filter(uid(F)) {
						film_name: name@en
					}
					performance.character {
						character: name@en
					}
				}
			}
		}`,
		resp: `{
			"peterJ": [
				{
					"character": "Derek",
					"film_name": "Bad Taste",
					"name": "Peter Jackson"
				},
				{
					"character": "Mercenary On Boat",
					"film_name": "The Lord of the Rings: The Return of the King",
					"name": "Peter Jackson"
				},
				{
					"character": "Undertaker's Assistant",
					"film_name": "Dead Alive",
					"name": "Peter Jackson"
				},
				{
					"character": "Bum Outside Theater",
					"film_name": "Heavenly Creatures",
					"name": "Peter Jackson"
				},
				{
					"character": "Robert",
					"film_name": "Bad Taste",
					"name": "Peter Jackson"
				},
				{
					"character": "Rohirrim Warrior",
					"film_name": "The Lord of the Rings: The Two Towers",
					"name": "Peter Jackson"
				},
				{
					"character": "Albert Dreary",
					"film_name": "The Lord of the Rings: The Fellowship of the Ring",
					"name": "Peter Jackson"
				},
				{
					"character": "Gunner #1",
					"film_name": "King Kong",
					"name": "Peter Jackson"
				},
				{
					"character": "Prospector #4",
					"film_name": "The Valley",
					"name": "Peter Jackson"
				},
				{
					"character": "Man with Piercings",
					"film_name": "The Frighteners",
					"name": "Peter Jackson"
				},
				{
					"character": "Man at Pharmacy",
					"film_name": "The Lovely Bones",
					"name": "Peter Jackson"
				}
			]
		}`,
	},
	{
		query: `{
			var(func: allofterms(name@en, "Taraji Henson")) {
				actor.film {
					F as performance.film {
						G as genre
					}
				}
			}

			Taraji_films_by_genre(func: uid(G)) {
				genre_name : name@en
				films : ~genre @filter(uid(F)) {
					film_name : name@en
				}
			}
		}`,
		resp: `{
			"Taraji_films_by_genre": [
				{
					"genre_name": "Chase Movie",
					"films": [
						{
							"film_name": "Date Night"
						}
					]
				},
				{
					"genre_name": "Black comedy",
					"films": [
						{
							"film_name": "Smokin' Aces"
						}
					]
				},
				{
					"genre_name": "Television film",
					"films": [
						{
							"film_name": "Satan's School for Girls"
						}
					]
				},
				{
					"genre_name": "Romantic comedy",
					"films": [
						{
							"film_name": "Date Night"
						},
						{
							"film_name": "I Can Do Bad All by Myself"
						},
						{
							"film_name": "Larry Crowne"
						}
					]
				},
				{
					"genre_name": "Musical comedy",
					"films": [
						{
							"film_name": "I Can Do Bad All by Myself"
						}
					]
				},
				{
					"genre_name": "Musical Drama",
					"films": [
						{
							"film_name": "I Can Do Bad All by Myself"
						}
					]
				},
				{
					"genre_name": "Drama",
					"films": [
						{
							"film_name": "The Good Doctor"
						},
						{
							"film_name": "The Curious Case of Benjamin Button"
						},
						{
							"film_name": "Smokin' Aces"
						},
						{
							"film_name": "The Family That Preys"
						},
						{
							"film_name": "Satan's School for Girls"
						},
						{
							"film_name": "Not Easily Broken"
						},
						{
							"film_name": "I Can Do Bad All by Myself"
						},
						{
							"film_name": "Larry Crowne"
						}
					]
				},
				{
					"genre_name": "Comedy-drama",
					"films": [
						{
							"film_name": "The Family That Preys"
						},
						{
							"film_name": "I Can Do Bad All by Myself"
						}
					]
				},
				{
					"genre_name": "Cult film",
					"films": [
						{
							"film_name": "Satan's School for Girls"
						}
					]
				},
				{
					"genre_name": "Crime Fiction",
					"films": [
						{
							"film_name": "Smokin' Aces"
						},
						{
							"film_name": "Date Night"
						},
						{
							"film_name": "Satan's School for Girls"
						}
					]
				},
				{
					"genre_name": "Comedy",
					"films": [
						{
							"film_name": "Smokin' Aces"
						},
						{
							"film_name": "Date Night"
						},
						{
							"film_name": "The Family That Preys"
						},
						{
							"film_name": "Not Easily Broken"
						},
						{
							"film_name": "I Can Do Bad All by Myself"
						},
						{
							"film_name": "Larry Crowne"
						}
					]
				},
				{
					"genre_name": "Romance Film",
					"films": [
						{
							"film_name": "The Curious Case of Benjamin Button"
						},
						{
							"film_name": "Date Night"
						},
						{
							"film_name": "The Family That Preys"
						},
						{
							"film_name": "Not Easily Broken"
						},
						{
							"film_name": "I Can Do Bad All by Myself"
						},
						{
							"film_name": "Larry Crowne"
						}
					]
				},
				{
					"genre_name": "Action Thriller",
					"films": [
						{
							"film_name": "Smokin' Aces"
						}
					]
				},
				{
					"genre_name": "Adventure Film",
					"films": [
						{
							"film_name": "The Curious Case of Benjamin Button"
						}
					]
				},
				{
					"genre_name": "Action Comedy",
					"films": [
						{
							"film_name": "Date Night"
						}
					]
				},
				{
					"genre_name": "Film adaptation",
					"films": [
						{
							"film_name": "Not Easily Broken"
						}
					]
				},
				{
					"genre_name": "Action/Adventure",
					"films": [
						{
							"film_name": "Date Night"
						}
					]
				},
				{
					"genre_name": "Mystery",
					"films": [
						{
							"film_name": "Satan's School for Girls"
						}
					]
				},
				{
					"genre_name": "Action Film",
					"films": [
						{
							"film_name": "Smokin' Aces"
						},
						{
							"film_name": "Date Night"
						}
					]
				},
				{
					"genre_name": "Horror",
					"films": [
						{
							"film_name": "Satan's School for Girls"
						}
					]
				},
				{
					"genre_name": "Thriller",
					"films": [
						{
							"film_name": "The Good Doctor"
						},
						{
							"film_name": "Smokin' Aces"
						},
						{
							"film_name": "Date Night"
						}
					]
				},
				{
					"genre_name": "Backstage Musical",
					"films": [
						{
							"film_name": "I Can Do Bad All by Myself"
						}
					]
				},
				{
					"genre_name": "Family Drama",
					"films": [
						{
							"film_name": "The Family That Preys"
						}
					]
				},
				{
					"genre_name": "Screwball comedy",
					"films": [
						{
							"film_name": "Date Night"
						}
					]
				}
			]
		}`,
	},
	{
		query: `{
			q(func: allofterms(name@en, "Ang Lee")) {
				director.film {
					name@en
					num_actors as count(starring)
				}
				most_actors : max(val(num_actors))
			}
		}`,
		resp: `{
			"q": [
				{
					"director.film": [
						{
							"name@en": "Ride with the Devil",
							"count(starring)": 10
						},
						{
							"name@en": "Crouching Tiger, Hidden Dragon",
							"count(starring)": 31
						},
						{
							"name@en": "Taking Woodstock",
							"count(starring)": 130
						},
						{
							"name@en": "The Ice Storm",
							"count(starring)": 45
						},
						{
							"name@en": "Eat Drink Man Woman",
							"count(starring)": 16
						},
						{
							"name@en": "Life of Pi",
							"count(starring)": 42
						},
						{
							"name@en": "Lust, Caution",
							"count(starring)": 16
						},
						{
							"name@en": "The Wedding Banquet",
							"count(starring)": 15
						},
						{
							"name@en": "The Hire: Chosen",
							"count(starring)": 1
						},
						{
							"name@en": "Brokeback Mountain",
							"count(starring)": 54
						},
						{
							"name@en": "Sense and Sensibility",
							"count(starring)": 15
						},
						{
							"name@en": "Hulk",
							"count(starring)": 15
						}
					],
					"most_actors": 130
				}
			]
		}`,
	},
	{
		query: `{
			ID as var(func: allofterms(name@en, "Steven Spielberg")) {
			}

			avs(func: uid(ID)) @normalize {
				name : name@en
			}
		}`,
		resp: `{
			"avs": [
				{
					"name": "Steven Spielberg"
				}
			]
		}`,
	},
	{
		query: `{
			ID as var(func: allofterms(name@en, "Steven")) {
				director.film {
					num_actors as count(starring)
				}
				average as avg(val(num_actors))
			}

			avs(func: uid(ID), orderdesc: val(average)) @filter(ge(val(average), 40)) @normalize {
				name : name@en
				average_actors : val(average)
				num_films : count(director.film)
			}
		}`,
		resp: `{
			"avs": [
				{
					"name": "Steven Spielberg",
					"average_actors": 51.733333,
					"num_films": 30
				},
				{
					"name": "Steven Zaillian",
					"average_actors": 40,
					"num_films": 3
				}
			]
		}`,
	},
	{
		query: `{
			var(func:allofterms(name@en, "Jean-Pierre Jeunet")) {
				name@en
				films as director.film {
					stars as count(starring)
					directors as count(~director.film)
					ratio as math(stars / directors)
				}
			}

			best_ratio(func: uid(films), orderdesc: val(ratio)){
				name@en
				stars_per_director : val(ratio)
				num_stars : val(stars)
			}
		}`,
		resp: `{
			"best_ratio": [
				{
					"name@en": "A Very Long Engagement",
					"stars_per_director": 78,
					"num_stars": 78
				},
				{
					"name@en": "Micmacs",
					"stars_per_director": 72,
					"num_stars": 72
				},
				{
					"name@en": "Amélie",
					"stars_per_director": 68,
					"num_stars": 68
				},
				{
					"name@en": "The City of Lost Children",
					"stars_per_director": 43,
					"num_stars": 87
				},
				{
					"name@en": "The Young and Prodigious Spivet",
					"stars_per_director": 15,
					"num_stars": 15
				},
				{
					"name@en": "Alien: Resurrection",
					"stars_per_director": 15,
					"num_stars": 15
				},
				{
					"name@en": "Delicatessen",
					"stars_per_director": 8,
					"num_stars": 17
				},
				{
					"name@en": "Things I Like, Things I Don't Like",
					"stars_per_director": 2,
					"num_stars": 2
				}
			]
		}`,
	},
	{
		query: `{
			var(func:allofterms(name@en, "Steven Spielberg")) {
				director.film @groupby(genre) {
					a as count(uid)
				}
			}

			byGenre(func: uid(a), orderdesc: val(a)) {
				name@en
				num_movies : val(a)
			}
		}`,
		resp: `{
			"byGenre": [
				{
					"name@en": "Drama",
					"num_movies": 19
				},
				{
					"name@en": "Adventure Film",
					"num_movies": 14
				},
				{
					"name@en": "Action Film",
					"num_movies": 11
				},
				{
					"name@en": "Thriller",
					"num_movies": 10
				},
				{
					"name@en": "Science Fiction",
					"num_movies": 8
				},
				{
					"name@en": "War film",
					"num_movies": 6
				},
				{
					"name@en": "Comedy",
					"num_movies": 5
				},
				{
					"name@en": "Family",
					"num_movies": 4
				},
				{
					"name@en": "Historical fiction",
					"num_movies": 4
				},
				{
					"name@en": "Mystery",
					"num_movies": 4
				},
				{
					"name@en": "Biographical film",
					"num_movies": 4
				},
				{
					"name@en": "Fantasy",
					"num_movies": 4
				},
				{
					"name@en": "Costume Adventure",
					"num_movies": 3
				},
				{
					"name@en": "Action/Adventure",
					"num_movies": 3
				},
				{
					"name@en": "Horror",
					"num_movies": 3
				},
				{
					"name@en": "Adventure Comedy",
					"num_movies": 3
				},
				{
					"name@en": "Film adaptation",
					"num_movies": 3
				},
				{
					"name@en": "Crime Fiction",
					"num_movies": 2
				},
				{
					"name@en": "Romance Film",
					"num_movies": 2
				},
				{
					"name@en": "Future noir",
					"num_movies": 2
				},
				{
					"name@en": "Epic film",
					"num_movies": 2
				},
				{
					"name@en": "Chase Movie",
					"num_movies": 1
				},
				{
					"name@en": "Disaster Film",
					"num_movies": 1
				},
				{
					"name@en": "Dystopia",
					"num_movies": 1
				},
				{
					"name@en": "Doomsday film",
					"num_movies": 1
				},
				{
					"name@en": "Adventure",
					"num_movies": 1
				},
				{
					"name@en": "Children's Fantasy",
					"num_movies": 1
				},
				{
					"name@en": "Action Comedy",
					"num_movies": 1
				},
				{
					"name@en": "Comedy-drama",
					"num_movies": 1
				},
				{
					"name@en": "Political drama",
					"num_movies": 1
				},
				{
					"name@en": "Coming of age",
					"num_movies": 1
				},
				{
					"name@en": "Airplanes and airports",
					"num_movies": 1
				},
				{
					"name@en": "Road movie",
					"num_movies": 1
				},
				{
					"name@en": "Fantasy Adventure",
					"num_movies": 1
				},
				{
					"name@en": "Childhood Drama",
					"num_movies": 1
				},
				{
					"name@en": "Screwball comedy",
					"num_movies": 1
				},
				{
					"name@en": "Children's/Family",
					"num_movies": 1
				},
				{
					"name@en": "Crime",
					"num_movies": 1
				},
				{
					"name@en": "Animation",
					"num_movies": 1
				},
				{
					"name@en": "Historical period drama",
					"num_movies": 1
				},
				{
					"name@en": "Romantic comedy",
					"num_movies": 1
				},
				{
					"name@en": "Suspense",
					"num_movies": 1
				},
				{
					"name@en": "Alien Film",
					"num_movies": 1
				},
				{
					"name@en": "Political thriller",
					"num_movies": 1
				},
				{
					"name@en": "Film noir",
					"num_movies": 1
				},
				{
					"name@en": "Mystery film",
					"num_movies": 1
				}
			]
		}`,
	},
}

func Test1Million(t *testing.T) {
	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	if err != nil {
		t.Fatalf("Error while getting a dgraph client: %v", err)
	}

	for _, tt := range tc {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		resp, err := dg.NewTxn().Query(ctx, tt.query)
		cancel()

		if ctx.Err() == context.DeadlineExceeded {
			t.Fatal("aborting test due to query timeout")
		}
		require.NoError(t, err)

		testutil.CompareJSON(t, tt.resp, string(resp.Json))
	}
}
