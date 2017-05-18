import { connect } from "react-redux";

import FrameQueryEditor from "../components/FrameQueryEditor";
import {
  runQuery
} from "../actions";

const mapStateToProps = state => ({
});

const mapDispatchToProps = dispatch => ({
  handleRunQuery(query, frameId, done) {
    return dispatch(runQuery(query, frameId))
      .then(done);
  }
});

export default connect(mapStateToProps, mapDispatchToProps)(FrameQueryEditor);
