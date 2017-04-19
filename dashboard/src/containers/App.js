import React from "react";
import { connect } from "react-redux";
import screenfull from "screenfull";
import { Alert } from "react-bootstrap";

import NavbarContainer from "../containers/NavbarContainer";
import PreviousQueryListContainer from "./PreviousQueryListContainer";
import Editor from "./Editor";
import Response from "./Response";
import {
  updateFullscreen,
  getQuery,
  updateInitialQuery,
  queryFound,
  selectQuery,
  runQuery
} from "../actions";
import { readCookie, eraseCookie } from './Helpers';

import "../assets/css/App.css";

class App extends React.Component {
  enterFullScreen = updateFullscreen => {
    if (!screenfull.enabled) {
      return;
    }

    document.addEventListener(screenfull.raw.fullscreenchange, () => {
      updateFullscreen(screenfull.isFullscreen);
    });
    screenfull.request(document.getElementById("response"));
  };

  render = () => {
    return (
      <div>
        <NavbarContainer />
        <div className="container-fluid">
          <div className="row justify-content-md-center">
            <div className="col-sm-12">
              <div className="col-sm-8 col-sm-offset-2">
                {!this.props.found &&
                  <Alert
                    ref={alert => {
                      this.alert = alert;
                    }}
                    bsStyle="danger"
                    onDismiss={() => {
                      this.props.queryFound(true);
                    }}
                  >
                    Couldn't find query with the given id.
                  </Alert>}
              </div>
              <div className="col-sm-5">
                <Editor />
                <PreviousQueryListContainer xs="hidden-xs" />
              </div>
              <div className="col-sm-7">
                <label style={{ marginLeft: "5px" }}> Response </label>
                {screenfull.enabled &&
                  <div
                    title="Enter full screen mode"
                    className="pull-right App-fullscreen"
                    onClick={() => this.enterFullScreen(this.props.updateFs)}
                  >
                    <span
                      className="App-fs-icon glyphicon glyphicon-glyphicon glyphicon-resize-full"
                    />
                  </div>}
                <Response />
                <PreviousQueryListContainer xs="visible-xs-block" />
              </div>
            </div>
          </div>
        </div>
      </div>
    );
  };

  componentDidMount = () => {
    const { handleSelectQuery, handleRunQuery } = this.props;

    let id = this.props.match.params.id;
    if (id !== undefined) {
      this.props.getQuery(id);
    }

    // If playQuery cookie is set, run the query and erase the cookie
    // The cookie is used to communicate the query string between docs and play
    const playQuery = readCookie('playQuery');
    if (playQuery) {
      const queryString = decodeURI(playQuery);
      handleSelectQuery(queryString);
      handleRunQuery(queryString).then(() => {
        eraseCookie('playQuery', { crossDomain: true });
      });
    }
  };

  componentWillReceiveProps = nextProps => {
    if (!nextProps.found) {
      // Lets auto close the alert after 2 secs.
      setTimeout(
        () => {
          this.props.queryFound(true);
        },
        3000
      );
    }
  };
}

const mapStateToProps = state => ({
  found: state.share.found
});

const mapDispatchToProps = dispatch => ({
  updateFs: fs => {
    dispatch(updateFullscreen(fs));
  },
  getQuery: id => {
    dispatch(getQuery(id));
  },
  updateInitialQuery: () => {
    dispatch(updateInitialQuery());
  },
  queryFound: found => {
    dispatch(queryFound(found));
  },
  handleSelectQuery: (queryText) => {
    dispatch(selectQuery(queryText));
  },
  handleRunQuery: (query) => {
    return dispatch(runQuery(query));
  }
});

export default connect(mapStateToProps, mapDispatchToProps)(App);
