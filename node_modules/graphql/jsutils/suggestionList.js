"use strict";

Object.defineProperty(exports, "__esModule", {
  value: true
});
exports.default = suggestionList;

/**
 * Given an invalid input string and a list of valid options, returns a filtered
 * list of valid options sorted based on their similarity with the input.
 */
function suggestionList(input, options) {
  var optionsByDistance = Object.create(null);
  var lexicalDistance = new LexicalDistance(input);
  var threshold = Math.floor(input.length * 0.4) + 1;

  for (var _i2 = 0; _i2 < options.length; _i2++) {
    var option = options[_i2];
    var distance = lexicalDistance.measure(option, threshold);

    if (distance !== undefined) {
      optionsByDistance[option] = distance;
    }
  }

  return Object.keys(optionsByDistance).sort(function (a, b) {
    var distanceDiff = optionsByDistance[a] - optionsByDistance[b];
    return distanceDiff !== 0 ? distanceDiff : a.localeCompare(b);
  });
}
/**
 * Computes the lexical distance between strings A and B.
 *
 * The "distance" between two strings is given by counting the minimum number
 * of edits needed to transform string A into string B. An edit can be an
 * insertion, deletion, or substitution of a single character, or a swap of two
 * adjacent characters.
 *
 * Includes a custom alteration from Damerau-Levenshtein to treat case changes
 * as a single edit which helps identify mis-cased values with an edit distance
 * of 1.
 *
 * This distance can be useful for detecting typos in input or sorting
 */


var LexicalDistance = /*#__PURE__*/function () {
  function LexicalDistance(input) {
    this._input = input;
    this._inputLowerCase = input.toLowerCase();
    this._inputArray = stringToArray(this._inputLowerCase);
    this._rows = [new Array(input.length + 1).fill(0), new Array(input.length + 1).fill(0), new Array(input.length + 1).fill(0)];
  }

  var _proto = LexicalDistance.prototype;

  _proto.measure = function measure(option, threshold) {
    if (this._input === option) {
      return 0;
    }

    var optionLowerCase = option.toLowerCase(); // Any case change counts as a single edit

    if (this._inputLowerCase === optionLowerCase) {
      return 1;
    }

    var a = stringToArray(optionLowerCase);
    var b = this._inputArray;

    if (a.length < b.length) {
      var tmp = a;
      a = b;
      b = tmp;
    }

    var aLength = a.length;
    var bLength = b.length;

    if (aLength - bLength > threshold) {
      return undefined;
    }

    var rows = this._rows;

    for (var j = 0; j <= bLength; j++) {
      rows[0][j] = j;
    }

    for (var i = 1; i <= aLength; i++) {
      var upRow = rows[(i - 1) % 3];
      var currentRow = rows[i % 3];
      var smallestCell = currentRow[0] = i;

      for (var _j = 1; _j <= bLength; _j++) {
        var cost = a[i - 1] === b[_j - 1] ? 0 : 1;
        var currentCell = Math.min(upRow[_j] + 1, // delete
        currentRow[_j - 1] + 1, // insert
        upRow[_j - 1] + cost // substitute
        );

        if (i > 1 && _j > 1 && a[i - 1] === b[_j - 2] && a[i - 2] === b[_j - 1]) {
          // transposition
          var doubleDiagonalCell = rows[(i - 2) % 3][_j - 2];
          currentCell = Math.min(currentCell, doubleDiagonalCell + 1);
        }

        if (currentCell < smallestCell) {
          smallestCell = currentCell;
        }

        currentRow[_j] = currentCell;
      } // Early exit, since distance can't go smaller than smallest element of the previous row.


      if (smallestCell > threshold) {
        return undefined;
      }
    }

    var distance = rows[aLength % 3][bLength];
    return distance <= threshold ? distance : undefined;
  };

  return LexicalDistance;
}();

function stringToArray(str) {
  var strLength = str.length;
  var array = new Array(strLength);

  for (var i = 0; i < strLength; ++i) {
    array[i] = str.charCodeAt(i);
  }

  return array;
}
