import { connect } from "react-redux";

import Scratchpad from "../components/Scratchpad";

import { deleteScratchpadEntries } from "../actions";

const mapStateToProps = state => ({
    scratchpad: state.scratchpad
});

const mapDispatchToProps = dispatch => ({
    deleteAllEntries: () => {
        dispatch(deleteScratchpadEntries());
    }
});

export default connect(mapStateToProps, mapDispatchToProps)(Scratchpad);
