//go:build integration2

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"testing"
	"time"

	"github.com/hypermodeinc/dgraph/v25/dgraphapi"
	"github.com/hypermodeinc/dgraph/v25/dgraphtest"
	"github.com/hypermodeinc/dgraph/v25/x"
	"github.com/stretchr/testify/require"
)

func TestGraphqlSchema(t *testing.T) {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(2).WithNumZeros(1).
		WithACL(time.Hour).WithReplicas(1).
		WithGraphqlLambdaURL("http://127.0.0.1/lambda") // dummy URL
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()
	require.NoError(t, c.Start())

	hc, err := c.HTTPClient()
	require.NoError(t, err)
	require.NoError(t, hc.LoginIntoNamespace(dgraphapi.DefaultUser,
		dgraphapi.DefaultPassword, x.RootNamespace))

	// DGRAPHCORE-329
	//nolint:lll
	sch1 := `type City @auth(
		query: { or: [
			{ rule: "{$Role: { eq: \"ADMIN\" }}"},
			{ rule: """
				query($CITYID: String!) {
					queryCity (filter: {
						id: { eq: $CITYID }
					}){
						name
					}
				}"""
			}]
		},

		add: { rule: "{$Role: { eq: \"ADMIN\" }}"},
		update: { rule: "{$Role: { eq: \"ADMIN\" }}"},
	){
		id: ID!
		name: String! @search(by: [hash, trigram])
		state: String!
		country: String! @search(by: [hash, trigram])
		current_weather: String! @lambda
		desc: String! @lambda
	}

		type Query {
		citiesByName(name: String!): [City] @lambda
	}

	# Dgraph.Authorization {"VerificationKey":"secretkey","Header":"X-Test-Auth","Namespace":"https://xyz.io/jwt/claims","Algo":"HS256","Audience":["aud"]}`
	require.NoError(t, hc.UpdateGQLSchema(sch1))

	params := dgraphapi.GraphQLParams{
		Query: `query {
			queryCity(filter: { id: { eq: 0 } }) {
				name
			}
		}`,
	}
	_, err = hc.RunGraphqlQuery(params, false)
	require.NoError(t, err)

	params = dgraphapi.GraphQLParams{
		Query: `query {
			queryCity(filter: { id: "0" }) {
				name
			}
		}`,
	}
	_, err = hc.RunGraphqlQuery(params, false)
	require.NoError(t, err)

	// DGRAPHCORE-341
	//nolint:lll
	sch2 := `# #####################################################################
	# ##########################  GENERAL  ################################
	# #####################################################################

	enum Permission {
	  VIEWER
	}

	"""
	Tenant scopes resources to a tenant.
	"""
	type Tenant {
		label: String! @id @search(by: [hash])
	  hasUserRole: [UserRole]
	  hasConfiguration: TenantConfig @custom(http: {
		url: "https://config.eu.onis.test.onmi.design/api/tenant?label=$label",
		method: GET
	  })
	}

	type TenantConfig @remote {
	  name: String
		label: String!
		graphUrl: String
		privacyUrl: String
		termsUrl: String
		licenseUrl: String
		lukaUrl: String
	  withProgramInfo: [ProgramInfo]
	}

	"""
	Member access roles.
	"""
	type UserRole {
	  id: ID!
	  permission: Permission!
	  email: String! @search(by:[hash])
	  createdAt: DateTime
	}

	"""
	Provider defines a data provider.
	"""
	type Provider
	  @generate(
		query: { get: true, query: true, aggregate: false }
		mutation: { add: true, update: true, delete: false }
	  )
	  @lambdaOnMutate(add: true, update: true, delete: false)
	  @auth(
		add: {
		  rule: """
		  #* client must match provider
		  query ($client_id: String!) {
			  queryProvider (filter: {name: {eq: $client_id}}){
				  __typename
			  }
		  }
		  """
		}
		update: {
		  rule: """
		  #* client must match provider
		  query ($client_id: String!) {
			  queryProvider (filter: {name: {eq: $client_id}}){
				  __typename
			  }
		  }
		  """
		}
	  ) {
	  id: ID!
	  name: String! @id
	  createdAt: DateTime
	  updatedAt: DateTime
	  hasData: [ProviderData] @hasInverse(field: forProvider)
	  hasSource: [Source] @hasInverse(field: forProvider)
	  hasAuthorizationRecord: [AuthorizationRecord]
	}

	"""
	User defines a user for which data is collected.
	"""
	type User
	  @generate(
		query: { get: true, query: true, aggregate: true }
		mutation: { add: true, update: true, delete: false }
	  )
	  @lambdaOnMutate(add: true, update: true, delete: false)
	  @auth(
		add: {
		  or: [
			# * user is an admin
			{ rule: """{ $is_admin: { eq: true } }""" }
			#* by user claim
			{
			  rule: """
			  #* client must match user
			  query ($user_id: String!) {
				  queryUser (filter: {userId: {eq: $user_id}}){
					  __typename
				  }
			  }
			  """
			}
		  ]
		}
		update: {
		  or: [
			# * user is an admin
			{ rule: """{ $is_admin: { eq: true } }""" }
			#* by user claim
			{
			  rule: """
			  #* client must match user
			  query ($user_id: String!) {
				  queryUser (filter: {userId: {eq: $user_id}}){
					  __typename
				  }
			  }
			  """
			}
		  ]
		}
		query: {
		  or: [
			# * user is an admin
			{ rule: "{ $is_admin: { eq: true } }" }
			#* client must match user
			{
			  rule: """
			  query ($user_id: String!) {
				queryUser (filter: {userId: {eq: $user_id}}){
					__typename
				}
			  }
			  """
			}
			#* client must have label
			{
			  rule: """
			  query ($label: [String]) {
				queryUser {
				  hasProgram (filter: {label: {in: $label}}){
					__typename
				  }
				}
			  }
			  """
			}
		  ]
		}
	  ) {
	  id: ID!
	  userId: String! @id
	  createdAt: DateTime
	  updatedAt: DateTime
	  # hasData: [UserData] @hasInverse(field: forUser)
	  hasAuthorizationRecord: [AuthorizationRecord]
	  hasProgram: [Program] @hasInverse(field: forUser)
	  hasFitbitActivityDailies: [FitbitActivityDailies] @hasInverse(field: forUser)
	  hasLukaDailies: [LukaDailies] @hasInverse(field: forUser)
	}

	"""
	ProviderSource identifies the source for the data provider.
	"""
	type Source
	  @generate(
		query: { get: true, query: true, aggregate: true }
		mutation: { add: true, update: true, delete: false }
	  )
	  @auth(
		add: {
		  rule: """
		  #* client must match provider
		  query {
			  querySource {
				forProvider{
				  __typename
				}
			  }
		  }
		  """
		}
		update: {
		  rule: """
		  #* client must match provider
		  query {
			  querySource {
				forProvider{
				  __typename
				}
			  }
		  }
		  """
		}
	  ) {
	  id: ID!
	  uri: String! @id
	  forProvider: Provider! @hasInverse(field: hasSource)
	  hasData: [SourceData] @hasInverse(field: fromSource)
	}

	"""
	Authorization record stores metadata about the user's last access grants.
	"""
	type AuthorizationRecord
	  @generate(
		query: { get: true, query: true, aggregate: false }
		mutation: { add: true, update: true, delete: false }
	  )
	  @auth(
		add: {
		  and: [
			{
			  rule: """
			  #* user must match token
			  query {
				  queryAuthorizationRecord {
					  forUser {
						  __typename
					  }
				  }
			  }
			  """
			}
			{
			  rule: """
			  #* provider must match token
			  query {
				  queryAuthorizationRecord {
					  forProvider {
						  __typename
					  }
				  }
			  }
			  """
			}
		  ]
		}
		update: {
		  and: [
			{
			  rule: """
			  #* user must match token
			  query {
				  queryAuthorizationRecord {
					  forUser {
						  __typename
					  }
				  }
			  }
			  """
			}
			{
			  rule: """
			  #* provider must match token
			  query {
				  queryAuthorizationRecord {
					  forProvider {
						  __typename
					  }
				  }
			  }
			  """
			}
		  ]
		}
	  ) {
	  id: ID!
	  recordId: String! @id
	  forUser: User! @hasInverse(field: hasAuthorizationRecord)
	  forProvider: Provider! @hasInverse(field: hasAuthorizationRecord)
	  authorizedAt: DateTime
	  subscribedAt: DateTime
	  revokedAt: DateTime
	}

	"""
	Implementable by types that can be updated.
	"""
	interface Updateable
	  @generate(
		query: { get: false, query: false, aggregate: false }
		mutation: { add: false, update: false, delete: false }
	  ) {
	  """
	  Last time the state was updated.
	  """
	  updatedAt: DateTime
	  """
	  Service name that last updated state.
	  """
	  updatedBy: User
	}

	"""
	ProviderData represent data belonging to a provider.
	"""
	interface ProviderData
	  @generate(
		query: { get: false, query: true, aggregate: true }
		mutation: { add: false, update: false, delete: false }
	  ) {
	  forProvider: Provider!
	}

	"""
	ProviderSourceData identifies a resource from which data is retrieved. i.e., from event, database, url.
	"""
	interface SourceData
	  @generate(
		query: { get: false, query: true, aggregate: true }
		mutation: { add: false, update: false, delete: false }
	  ) {
	  fromSource: Source!
	}

	"""
	UserData represent data belonging to a user.
	"""
	interface UserData
	  @generate(
		query: { get: false, query: true, aggregate: true }
		mutation: { add: false, update: false, delete: false }
	  )
	  @auth(
		add: {
		  or: [
			# * user is an admin
			{ rule: """{ $is_admin: { eq: true } }""" }
			#* by user claim
			{
			  rule: """
			  #* client must match user
			  query ($user_id: String!) {
				queryUserData{
				  forUser (filter: {userId: {eq: $user_id}}){
					  __typename
				  }
				}
			  }
			  """
			}
		  ]
		}
		update: {
		  or: [
			# * user is an admin
			{ rule: """{ $is_admin: { eq: true } }""" }
			#* by user claim
			{
			  rule: """
			  #* client must match user
			  query ($user_id: String!) {
				queryUserData{
				  forUser (filter: {userId: {eq: $user_id}}){
					  __typename
				  }
				}
			  }
			  """
			}
		  ]
		}
		query: {
		  or: [
			# * user is an admin
			{ rule: """{ $is_admin: { eq: true } }""" }
			#* by user claim
			{
			  rule: """
			  query ($user_id: String!) {
				queryUserData {
				  forUser (filter: {userId: {eq: $user_id}}){
					  __typename
				  }
				}
			  }
			  """
			}
			#* by program label claim
			{
			  rule: """
			  query ($label: [String]) {
				queryUserData {
				  forUser {
					hasProgram (filter: {label: {in: $label}}){
					  __typename
					}
				  }
				}
			  }
			  """
			}
			#* by tenant permission
			{
			  rule: """
			  query ($email: String) {
				queryUserData {
				  inTenant {
					hasUserRole (filter: {email: {eq: $email} and: {permission: { in: [VIEWER]}}}){
					  __typename
					}
				  }
				}
			  }
			  """
			}
		  ]
		}
		delete: {
		  rule: """
		  #* client must match user
		  query ($user_id: String!) {
			queryUserData{
			  forUser (filter: {userId: {eq: $user_id}}){
				  __typename
			  }
			}
		  }
		  """
		}
	  ) {
	  forUser: User!
	  inTenant: Tenant
	}

	interface SharedUserData
	  @generate(
		query: { get: false, query: true, aggregate: true }
		mutation: { add: false, update: false, delete: false }
	  ) {
	  forUser: [User!]!
	}

	"""
	Interface for dailies.
	"""
	interface Dailies
	  @generate(
		query: { get: false, query: true, aggregate: true }
		mutation: { add: false, update: false, delete: false }
	  ) {
	  """
	  Summary date without time and timezone information.
	  """
	  date: DateTime! @search(by: [day])
	}

	# #####################################################################
	# ############################  LUKA  #################################
	# #####################################################################

	"""
	Place stores data for a visited place.
	"""
	type Place implements ProviderData & SharedUserData {
	  id: ID!
	  xId: String! @id
	  placeType: String
	  point: Point! @search
	  hasStop: [StorylineSegment]
	}

	"""
	Tracepoint stores sample GPS location along with other motion data.
	"""
	type Tracepoint implements ProviderData & UserData {
	  id: ID!
	  xId: String! @id
	  timestamp: DateTime! @search(by: [hour])
	  speed: Float
	  point: Point! @search
	  floor: Float
	  accuracy: Float
	  altitudeAccuracy: Float
	  altitude: Float
	  heading: Float
	  isMoving: Boolean
	  odometer: Float
	  activityType: String
	  activityConfidence: Float
	  batteryLevel: Float
	  batteryIsCharging: Boolean
	  devicePlatform: String
	  deviceManufacturer: String
	  deviceModel: String
	  deviceFramework: String
	  deviceVersion: String
	  deviceUUID: String!
	  deviceTimezone: String
	  deviceAppVersion: String
	  deviceLocale: String
	  deviceEmulator: Boolean
	  deviceFontScale: Float
	  """
	  Number of steps since start of day for the device timezone.
	  """
	  stepCount: Int
	  """
	  Number of steps since zero date (January 1, 1970 00:00:00).
	  """
	  stepsZero: Int
	  sampleType: String @search
	}

	"""
	Activity is a user activity captured during a stop or a movement.
	"""
	type LukaActivity implements ProviderData & UserData & Updateable {
	  id: ID!
	  xId: String! @id
	  startedAt: DateTime!
	  endedAt: DateTime!
	  duration: Int!
	  distance: Float!
	  activityType: String!
	  steps: Int!
	  inSegment: StorylineSegment @hasInverse(field: hasActivity)
	  hasTracepoint: [Tracepoint!]
	}

	"""
	SegmentType defines the storyline segment types used to denote a STOP/PLACE or identify travel segments (MOVE)
	"""
	enum SegmentType {
	  PLACE
	  MOVE
	  OFF
	}

	"""
	StorylineSegment defines Stop or Move activity segments within a user's storyline.
	"""
	type StorylineSegment implements ProviderData & UserData & Updateable {
	  id: ID!
	  xId: String! @id
	  segmentType: SegmentType! @search
	  startedAt: DateTime! @search(by: [hour])
	  endedAt: DateTime! @search(by: [hour])
	  transitionedFrom: StorylineSegment
	  transitionedTo: StorylineSegment @hasInverse(field: transitionedFrom)
	  hasActivity: [LukaActivity]
	  hasTracepoint: [Tracepoint]
	  inPlace: Place @hasInverse(field: hasStop)
	  distance: Int!
	  steps: Int!
	}

	"""
	LukaState stores data for processing user data in Luka. The state should be unique for a user's device.
	"""
	type LukaState implements ProviderData & UserData & Updateable
	  @withSubscription {
	  id: ID!
	  """
	  State ID should be unique for every user's device. I.e., hash of <userId + deviceUUID>.
	  """
	  xId: String! @id
	  """
	  The device that captured the location data.
	  """
	  deviceUUID: String!
	  """
	  The timestamp for the last updated location by API.
	  """
	  locationUpdatedAt: DateTime
	  """
	  The timestamp for the last location processed by the Places processor.
	  """
	  processPlacesFrom: DateTime
	  """
	  Processing parameter used by Places.
	  Stores processing parameters in a serialized format.
	  """
	  processPlacesVarsString: String
	  """
	  Indicates ongoing processing by Places for location updates.
	  Used in combination with acquiredByPlacesAt for distributed lock and TTL.
	  """
	  isAcquiredByPlaces: Boolean @search
	  """
	  Time of acquisition for processing by the Places processor.
	  Used in combination with isAcquiredByPlaces for distributed lock and TTL.
	  """
	  acquiredByPlacesAt: DateTime @search(by: [hour])
	}

	type LukaDailies implements ProviderData & UserData & Dailies & Updateable {
	  id: ID!
	  """
	  ID should be unique for every user's summary date.
	  """
	  xId: String! @id
	  """
	  The number of seconds in the day covered by the summary report.
	  """
	  duration: Int!
	  """
	  The number of steps taken in the day.
	  """
	  steps: Int
	  """
	  The distance covered in meters.
	  """
	  distance: Int
	  """
	  The lenght between the furthest places visited in the day in meters.
	  """
	  reach: Float
	  """
	  New visited places. A place is new if never visited prior to the summary date.
	  """
	  newPlaces: [Place]
	  """
	  Uncommon visited places. A place is Uncommon if not visited for more than three times prior to the summary date.
	  """
	  UncommonPlaces: [Place]
	  """
	  The total number of places visited in the day.
	  """
	  placeCount: Int
	  """
	  The number of new places visited in the day.
	  """
	  newPlaceCount: Int
	  """
	  The number of Uncommon places visited in the day.
	  """
	  UncommonPlaceCount: Int
	  """
	  Cummulative time (in seconds) stationary.
	  """
	  durationInPlaces: Int
	  """
	  Cummulative time (in seconds) spent in new places within the day.
	  """
	  durationInNewPlaces: Int
	  """
	  Cummulative time (in seconds) spent in Uncommon places within the day.
	  """
	  durationInUncommonPlaces: Int
	  """
	  Cummulative time (in seconds) moving.
	  """
	  durationMoving: Int
	}

	# #####################################################################
	# ###########################  FITBIT  ################################
	# #####################################################################

	"""
	Contains Fitbit activity data.
	Based on: https://api.fitbit.com/1/user/[user-id]/activities/date/[date].json.
	API reference: https://dev.fitbit.com/build/reference/web-api/activity/#get-daily-activity-summary
	"""
	type FitbitActivityDailies implements ProviderData & UserData & Dailies & SourceData & Updateable
	  @generate(
		query: { get: true, query: true, aggregate: true }
		mutation: { add: true, update: true, delete: true }
	  ) {
	  id: ID!
	  """
	  ID should be unique for every user's summary date.
	  """
	  xId: String! @id

	  activeScore: Int @search
	  """
	  The number of calories burned during the day for periods of time when the user was active above sedentary level.
	  This value is calculated minute by minute for minutes that fall under this criteria.
	  This includes BMR for those minutes as well as activity burned calories.
	  """
	  activityCalories: Int @search
	  """
	  Only BMR calories.
	  """
	  caloriesBMR: Int @search
	  """
	  Calorie burn goal (caloriesOut) represents either dynamic daily target from the premium trainer plan or manual calorie burn goal.
	  Goals are included to the response only for today and 21 days in the past.
	  """
	  caloriesOut: Int @search
	  """
	  Daily summary data and daily goals for elevation (elevation, floors) only included for users with a device with an altimeter.
	  """
	  elevation: Float
	  sedentaryMinutes: Int @search
	  lightlyActiveMinutes: Int @search
	  fairlyActiveMinutes: Int @search
	  veryActiveMinutes: Int @search
	  """
	  Daily summary data and daily goals for elevation (elevation, floors) only included for users with a device with an altimeter.
	  """
	  floors: Int
	  marginalCalories: Int
	  """
	  The steps field in activity log entires included only for activities that have steps (e.g. "Walking", "Running").
	  """
	  steps: Int @search
	  hasActivity: [FitbitActivity]
	  hasIntraday: [FitbitIntraday]
	}

	"""
	Stores Fitbit activity data.
	Based on: https://api.fitbit.com/1/user/[user-id]/activities/date/[date].json.
	API reference: https://dev.fitbit.com/build/reference/web-api/activity/#get-daily-activity-summary
	"""
	type FitbitActivity implements ProviderData & UserData & SourceData & Updateable
	  @generate(
		query: { get: true, query: true, aggregate: true }
		mutation: { add: true, update: true, delete: true }
	  ) {
	  id: ID!
	  """
	  Acitivity ID.
	  """
	  xId: String! @id
	  calories: Int @search
	  description: String
	  """
	  A frequent activity record contains the distance and duration values recorded the last time the activity was logged.
	  """
	  distance: Float @search
	  """
	  A frequent activity record contains the distance and duration values recorded the last time the activity was logged.
	  """
	  duration: Int @search
	  isFavorite: Boolean
	  """
	  Activity name
	  """
	  name: String @search
	  """
	  Activity start time. Hours and minutes in the format HH:mm.
	  """
	  startTime: DateTime @search
	  """
	  The steps field in activity log entires included only for activities that have steps (e.g. "Walking", "Running")
	  """
	  steps: Int @search
	  """
	  Parent activity
	  """
	  parent: FitbitActivity @hasInverse(field: child)
	  """
	  Child activity
	  """
	  child: FitbitActivity
	}

	enum FitbitIntradayField {
	  STEPS
	  HEARTRATE
	}

	"""
	Stores Fitbit activity intraday steps.
	Based on: GET /1/user/-/activities/steps/date/2014-09-01/1d.json.
	API reference: https://dev.fitbit.com/build/reference/web-api/activity/#get-daily-activity-summary
	"""
	type FitbitIntraday implements ProviderData & UserData & SourceData & Updateable
	  @generate(
		query: { get: true, query: true, aggregate: true }
		mutation: { add: true, update: true, delete: true }
	  ) {
	  id: ID!
	  """
	  Steps ID.
	  """
	  xId: String! @id
	  """
	  Intraday type
	  """
	  field: FitbitIntradayField! @search
	  """
	  Field value
	  """
	  value: Int @search
	  """
	  Reference to the activity dailies for the record.
	  """
	  inDailies: FitbitActivityDailies @hasInverse(field: hasIntraday)
	  """
	  The time for the record.
	  """
	  time: DateTime @search
	  """
	  The hour for the record.
	  """
	  hour: Int @search
	  """
	  The minute for the record.
	  """
	  minute: Int @search
	  """
	  The minute for the record.
	  """
	  second: Int @search
	  """
	  The interval unit. E.g., minute or hour.
	  """
	  intervalUnit: String
	  """
	  Record interval
	  """
	  interval: Float
	}

	"""
	Stores Fitbit sleep info.
	https://dev.fitbit.com/build/reference/web-api/sleep/
	GET https://api.fitbit.com/1.2/user/[user-id]/sleep/date/[date].json
	"""
	type FitbitSleepDailies implements ProviderData & UserData & Dailies & SourceData & Updateable
	  @generate(
		query: { get: true, query: true, aggregate: true }
		mutation: { add: true, update: true, delete: true }
	  ) {
	  id: ID!
	  """
	  ID should be unique for every user's summary date.
	  """
	  xId: String! @id
	  totalMinutesAsleep: Float @search
	  totalTimeInBed: Float @search
	  hasSleep: [FitbitSleep] @hasInverse(field: inDailies)
	}

	"""
	Stores Fitbit sleep info.
	https://dev.fitbit.com/build/reference/web-api/sleep/
	GET https://api.fitbit.com/1.2/user/[user-id]/sleep/date/[date].json
	"""
	type FitbitSleep implements ProviderData & UserData & SourceData & Updateable
	  @generate(
		query: { get: true, query: true, aggregate: true }
		mutation: { add: true, update: true, delete: true }
	  ) {
	  id: ID!
	  """
	  Sleep log ID.
	  """
	  xId: String! @id

	  minutesAfterWakeup: Float
	  minutesAsleep: Float
	  minutesAwake: Float
	  """
	  This is generally 0 for autosleep created sleep logs
	  """
	  minutesToFallAsleep: Float
	  startTime: DateTime @search
	  """
	  Value in minutes
	  """
	  timeInBed: Float
	  type: String
	  dateOfSleep: DateTime @search
	  """
	  value in milliseconds
	  """
	  duration: Int
	  efficiency: Float
	  isMainSleep: Boolean
	  """
	  Reference to the connected sleep dailies.
	  """
	  inDailies: FitbitSleepDailies
	}

	# #####################################################################
	# ###########################  ALIUS  ################################
	# #####################################################################

	"""
	Stores program information.
	"""
	type ProgramInfo @remote {
	  """
	  Program unique label.
	  """
	  label: String! @id
	  """
	  Program name.
	  """
	  name: String
	  """
	  Program duration in days.
	  """
	  duration: Int
	  """
	  Program description.
	  """
	  description: String
	  """
	  Program lunch date.
	  """
	  launchDate: DateTime
	  """
	  KPI: number of user targeted for inclusion.
	  """
	  targetUsers: Int
	  """
	  KPI: target date.
	  """
	  targetDate: DateTime @search

	  productId: String!
	  longDescription: String
		typeformUrl: String
		coverUrl: String
	  durationInDays: Int
	  inTenant: [TenantConfig!]!
	}

	"""
	Alius user program
	"""
	type Program implements UserData
	  @generate(
		query: { get: true, query: true, aggregate: true }
		mutation: { add: false, update: false, delete: false }
	  ) {
	  id: ID!
	  createdAt: DateTime @dgraph(pred: "createdAt") @search(by: [hour])
	  updatedAt: DateTime @dgraph(pred: "updatedAt") @search(by: [hour])
	  hasMessageWithFacets: [MessageWithFacets] @lambda
	  hasMessageWithFacetsAggr: MessageWithFacetsAggregate @lambda
	  hasMessage: [Message] @hasInverse(field: forProgram) @dgraph(pred: "messages")
	  userId: String! @search(by: [hash]) @dgraph(pred: "user")
	  label: [String] @search(by: [hash]) @dgraph(pred: "labels")
	  """
	  Base64 encoded source data. I.e., typeform response.
	  """
	  source: [String] @search(by: [hash]) @dgraph(pred: "source")

	  hasProgramInfo: ProgramInfo @custom(http: {
		url: "https://config.eu.onis.test.onmi.design/api/program?id=$id&label=$label&userId=$userId",
		method: GET
	  })
	}

	type ProgramWithFacets @remote {
	  id: ID!
	  createdAt: DateTime
	  updatedAt: DateTime
	  label: [String]
	  # facets/ edge attributes
	  scheduledAt: DateTime
	  targetTime: DateTime
	  day: Int
	  completedAt: DateTime
	  completed: Boolean
	  rating: Int
	}

	type ProgramWithFacetsAggregate @remote {
	  count: Int
	  completedCount: Int
	  completedAtMin: DateTime
	  completedAtMax: DateTime
	  scheduledAtMin: DateTime
	  scheduledAtMax: DateTime
	  targetTimeMin: DateTime
	  targetTimeMax: DateTime
	  ratingMin: Int
	  ratingMax: Int
	  ratingAvg: Float
	}

	type Message
	  @generate(
		query: { get: true, query: true, aggregate: true }
		mutation: { add: false, update: false, delete: false }
	  )
	  @auth(
		add: { rule: "{ $is_admin: { eq: true } }" }
		update: { rule: "{ $is_admin: { eq: true } }" }
		query: {
		  or: [
			#* user is an admin
			{ rule: "{ $is_admin: { eq: true } }" }
			#* client must match user
			{
			  rule: """
			  query ($user_id: String!) {
				queryMessage {
				  forProgram {
					forUser (filter: {userId: {eq: $user_id}}){
					  __typename
					}
				  }
				}
			  }
			  """
			}
			#* client must have label
			{
			  rule: """
			  query ($label: [String]) {
				queryMessage (filter: {labels: {in: $label}}){
				  __typename
				}
			  }
			  """
			}
		  ]
		}
	  ) {
	  id: ID!
	  xid: String @search(by: [hash]) @dgraph(pred: "id")
	  title: String @search(by: [fulltext]) @dgraph(pred: "title")
	  # titleEn: String @search(by: [fulltext]) @dgraph(pred: "title@en")
	  # titleDa: String @search(by: [fulltext]) @dgraph(pred: "title@da")
	  # titleCa: String @search(by: [fulltext]) @dgraph(pred: "title@ca")
	  # titleEs: String @search(by: [fulltext]) @dgraph(pred: "title@es")
	  # titlePl: String @search(by: [fulltext]) @dgraph(pred: "title@pl")

	  body: String @search(by: [fulltext]) @dgraph(pred: "body")
	  # bodyEn: String @search(by: [fulltext]) @dgraph(pred: "body@en")
	  # bodyDa: String @search(by: [fulltext]) @dgraph(pred: "body@da")
	  # bodyCa: String @search(by: [fulltext]) @dgraph(pred: "body@ca")
	  # bodyEs: String @search(by: [fulltext]) @dgraph(pred: "body@es")
	  # bodyPl: String @search(by: [fulltext]) @dgraph(pred: "body@pl")

	  link: String @search(by: [fulltext]) @dgraph(pred: "link")
	  # linkEn: String @search(by: [fulltext]) @dgraph(pred: "link@en")
	  # linkDa: String @search(by: [fulltext]) @dgraph(pred: "link@da")
	  # linkCa: String @search(by: [fulltext]) @dgraph(pred: "link@ca")
	  # linkEs: String @search(by: [fulltext]) @dgraph(pred: "link@es")
	  # linkPl: String @search(by: [fulltext]) @dgraph(pred: "link@pl")

	  question: String @search(by: [fulltext]) @dgraph(pred: "question")
	  # questionEn: String @search(by: [fulltext]) @dgraph(pred: "question@en")
	  # questionDa: String @search(by: [fulltext]) @dgraph(pred: "question@da")
	  # questionCa: String @search(by: [fulltext]) @dgraph(pred: "question@ca")
	  # questionEs: String @search(by: [fulltext]) @dgraph(pred: "question@es")
	  # questionPl: String @search(by: [fulltext]) @dgraph(pred: "question@pl")

	  answer: String @search(by: [hash]) @dgraph(pred: "answer")
	  # answerEn: String @search(by: [hash]) @dgraph(pred: "answer@en")
	  # answerDa: String @search(by: [hash]) @dgraph(pred: "answer@da")
	  # answerCa: String @search(by: [hash]) @dgraph(pred: "answer@ca")
	  # answerEs: String @search(by: [hash]) @dgraph(pred: "answer@es")
	  # answerPl: String @search(by: [hash]) @dgraph(pred: "answer@pl")

	  target: String @search(by: [hash]) @dgraph(pred: "target")
	  # targetEn: String @search(by: [hash]) @dgraph(pred: "target@en")
	  # targetDa: String @search(by: [hash]) @dgraph(pred: "target@da")
	  # targetCa: String @search(by: [hash]) @dgraph(pred: "target@ca")
	  # targetEs: String @search(by: [hash]) @dgraph(pred: "target@es")
	  # targetPl: String @search(by: [hash]) @dgraph(pred: "target@pl")

	  category: String @search(by: [hash]) @dgraph(pred: "category")
	  labels: [String] @search(by: [hash]) @dgraph(pred: "labels")

	  """
	  Linked programs.
	  """
	  forProgram: [Program]
	  forProgramWithFacets: [ProgramWithFacets] @lambda
	  forProgramWithFacetsAggr: ProgramWithFacetsAggregate @lambda
	}

	type MessageWithFacets @remote {
	  id: ID!
	  title: String
	  titleEN: String
	  titleDA: String
	  titleCA: String
	  titleES: String
	  titlePL: String
	  titleNL: String

	  body: String
	  bodyEN: String
	  bodyDA: String
	  bodyCA: String
	  bodyES: String
	  bodyPL: String
	  bodyNL: String

	  link: String
	  linkEN: String
	  linkDA: String
	  linkCA: String
	  linkES: String
	  linkPL: String
	  linkNL: String

	  question: String
	  questionEN: String
	  questionDA: String
	  questionCA: String
	  questionES: String
	  questionPL: String
	  questionNL: String

	  answer: String
	  answerEN: String
	  answerDA: String
	  answerCA: String
	  answerES: String
	  answerPL: String
	  answerNL: String

	  target: String
	  targetEN: String
	  targetDA: String
	  targetCA: String
	  targetES: String
	  targetPL: String
	  targetNL: String

	  category: String
	  labels: [String]

	  # facets/ edge attributes
	  scheduledAt: DateTime
	  targetTime: DateTime
	  day: Int
	  completedAt: DateTime
	  completed: Boolean
	  rating: Int
	}

	type MessageWithFacetsAggregate @remote {
	  count: Int
	  completedCount: Int
	  completedAtMin: DateTime
	  completedAtMax: DateTime
	  scheduledAtMin: DateTime
	  scheduledAtMax: DateTime
	  targetTimeMin: DateTime
	  targetTimeMax: DateTime
	  ratingMin: Int
	  ratingMax: Int
	  ratingAvg: Float
	}

	type Mutation {
	  markProgramMessage(
		programId: String!
		messageId: String!
		completed: Boolean!
		rating: Int!
	  ): String! @lambda
	}

	type Query {
	  fetchProgramInfo(label: String, lang: String): [ProgramInfo] @custom(http: {
		  url: "https://config.eu.onis.test.onmi.design/api/program?label=$label&lang=$lang",
		  method: GET
	  })

	  fetchTenant(label: String, lang: String): [TenantConfig] @custom(http: {
		  url: "https://config.eu.onis.test.onmi.design/api/tenant?label=$label&lang=$lang",
		  method: GET
	  })
	}

	# type Mutation {
	#   assignPermission(permittedUser String!, permission Permission!, labels [String!]!, makeAdmin Boolean) String! @lambda
	# }

	# Dgraph.Authorization {"JWKUrl":"https://www.googleapis.com/service_accounts/v1/jwk/securetoken@system.gserviceaccount.com", "Namespace": "https://onmi.design/claims", "Audience": ["onis-public"], "Header": "X-Auth-Token", "ClosedByDefault": true}`
	require.NoError(t, hc.UpdateGQLSchema(sch2))
}
