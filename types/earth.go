/*
 * Copyright 2016-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package types

import (
	"fmt"

	"github.com/golang/geo/s1"
)

// Helper functions for earth distances

// EarthRadiusMeters is the radius of the earth in meters (in a spherical earth model).
const EarthRadiusMeters = 1000 * 6371

// Length denotes a length on Earth
type Length float64

// EarthDistance converts an angle to distance on earth in meters.
func EarthDistance(angle s1.Angle) Length {
	return Length(angle.Radians() * EarthRadiusMeters)
}

// EarthAngle converts a to distance on earth in meters to an angle
func EarthAngle(dist float64) s1.Angle {
	return s1.Angle(dist / EarthRadiusMeters)
}

// Area denotes an area on Earth
type Area float64

// EarthArea converts an area on the unit sphere to an area on earth in sq. meters.
func EarthArea(a float64) Area {
	return Area(a * EarthRadiusMeters * EarthRadiusMeters)
}

// String converts the length to human readable units
func (l Length) String() string {
	if l > 1000 {
		return fmt.Sprintf("%.3f km", l/1000)
	} else if l < 1 {
		return fmt.Sprintf("%.3f cm", l*100)
	} else {
		return fmt.Sprintf("%.3f m", l)
	}
}

const km2 = 1000 * 1000
const cm2 = 100 * 100

// String converts the area to human readable units
func (a Area) String() string {
	if a > km2 {
		return fmt.Sprintf("%.3f km^2", a/km2)
	} else if a < 1 {
		return fmt.Sprintf("%.3f cm^2", a*cm2)
	} else {
		return fmt.Sprintf("%.3f m^2", a)
	}
}
