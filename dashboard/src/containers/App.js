import React from "react";
import { connect } from "react-redux";
import screenfull from "screenfull";

import NavBar from "../components/Navbar";
import PreviousQueryListContainer from "./PreviousQueryListContainer";
import Editor from "./Editor";
import Response from "./Response";
import { updateFullscreen } from "../actions";

import "../assets/css/App.css";

class App extends React.Component {
  // Verify this should still work.
  updateQuery = (e: Event) => {
    e.preventDefault();
    if (e.target instanceof HTMLElement) {
      this.setState({
        partial: false,
      });
    }
    window.scrollTo(0, 0);
  };

  resetState = () => {
    return {
      // TODO - Get hourglass back.
      resType: "hourglass",
    };
  };

  resetStateOnQuery = () => {
    this.setState(this.resetState());
  };

  // TODO - Verify that mutations and error messages show up fine.
  // renderResText = (type, text) => {
  //   this.setState({
  //     resType: type,
  //     response: text,
  //   });
  // };
  // TODO - Fix this. Get states from redux store.
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
        <NavBar />
        <div className="container-fluid">
          <div className="row justify-content-md-center">
            <div className="col-sm-12">
              <div className="col-sm-5">
                <Editor />
                <PreviousQueryListContainer xs="hidden-xs" />
              </div>
              <div className="col-sm-7">
                <label style={{ marginLeft: "5px" }}> Response </label>
                {screenfull.enabled &&
                  <div
                    className="pull-right App-fullscreen"
                    onClick={() => this.enterFullScreen(this.props.updateFs)}
                  >
                    <span
                      className="App-fs-icon glyphicon glyphicon-glyphicon glyphicon-resize-full"
                    />
                  </div>}
                <Response />
                <PreviousQueryListContainer xs="visible-xs-block" />

                {/*graph={this.state.graph}
                  resType={this.state.resType}
                  graphHeight={this.state.graphHeight}}
                /> 
                */
                }
              </div>
            </div>
          </div>
        </div>
      </div>
    );
  };
}

const mapDispatchToProps = dispatch => ({
  updateFs: fs => {
    dispatch(updateFullscreen(fs));
  },
});

export default connect(null, mapDispatchToProps)(App);
