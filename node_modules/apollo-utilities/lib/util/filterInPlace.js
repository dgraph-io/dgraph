"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
function filterInPlace(array, test, context) {
    var target = 0;
    array.forEach(function (elem, i) {
        if (test.call(this, elem, i, array)) {
            array[target++] = elem;
        }
    }, context);
    array.length = target;
    return array;
}
exports.filterInPlace = filterInPlace;
//# sourceMappingURL=filterInPlace.js.map