import { __assign } from "tslib";
import { useContext, useState, useRef, useEffect } from 'react';
import { getApolloContext } from '@apollo/react-common';
import { MutationData } from './data/MutationData';
export function useMutation(mutation, options) {
    var context = useContext(getApolloContext());
    var _a = useState({ called: false, loading: false }), result = _a[0], setResult = _a[1];
    var updatedOptions = options ? __assign(__assign({}, options), { mutation: mutation }) : { mutation: mutation };
    var mutationDataRef = useRef();
    function getMutationDataRef() {
        if (!mutationDataRef.current) {
            mutationDataRef.current = new MutationData({
                options: updatedOptions,
                context: context,
                result: result,
                setResult: setResult
            });
        }
        return mutationDataRef.current;
    }
    var mutationData = getMutationDataRef();
    mutationData.setOptions(updatedOptions);
    mutationData.context = context;
    useEffect(function () { return mutationData.afterExecute(); });
    return mutationData.execute(result);
}
//# sourceMappingURL=useMutation.js.map