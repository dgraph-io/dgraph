import { connect } from "react-redux";

import PreviousQueryList from "../components/PreviousQueryList";
import {
    selectQuery,
    deleteQuery,
    resetResponseState,
    deleteAllQueries
} from "../actions";

const mapStateToProps = state => ({
    queries: state.previousQueries
});

const mapDispatchToProps = dispatch => ({
    selectQuery: text => {
        dispatch(selectQuery(text));
    },
    deleteQuery: idx => {
        dispatch(deleteQuery(idx));
    },
    deleteAll: () => {
        dispatch(deleteAllQueries());
    },
    resetResponse: () => {
        dispatch(resetResponseState());
    }
});

export default connect(mapStateToProps, mapDispatchToProps)(PreviousQueryList);
