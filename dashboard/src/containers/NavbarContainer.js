import { connect } from "react-redux";

import NavBar from "../components/Navbar";
import { getShareId } from "../actions";

const mapStateToProps = state => ({
    shareId: state.share.id,
    query: state.query.text
});

const mapDispatchToProps = dispatch => ({
    getShareId: () => {
        dispatch(getShareId);
    }
});

export default connect(mapStateToProps, mapDispatchToProps)(NavBar);
