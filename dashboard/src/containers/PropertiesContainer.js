import { connect } from "react-redux";

import Properties from "../components/Properties";
import { addScratchpadEntry } from "../actions";

const mapStateToProps = (state, ownProps) => ({
    currentNode: state.interaction.node
});

const mapDispatchToProps = dispatch => ({
    addEntry: (uid, name) => {
        if (uid !== undefined) {
            dispatch(
                addScratchpadEntry({
                    uid,
                    name
                })
            );
        }
    }
});

export default connect(mapStateToProps, mapDispatchToProps)(Properties);
