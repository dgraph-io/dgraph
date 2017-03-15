import { connect } from "react-redux";

import PreviousQueryList from "../components/PreviousQueryList";
import { selectQuery, deleteQuery, resetResponseState } from "../actions";

import "../assets/css/App.css";

const mapStateToProps = state => ({
    queries: state.previousQueries,
});

const mapDispatchToProps = dispatch => ({
    selectQuery: text => {
        dispatch(selectQuery(text));
    },
    deleteQuery: idx => {
        dispatch(deleteQuery(idx));
    },
    resetResponse: () => {
        dispatch(resetResponseState());
    },
});

export default connect(mapStateToProps, mapDispatchToProps)(PreviousQueryList);
