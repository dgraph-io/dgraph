import React from "react";
import { connect } from "react-redux";
import classnames from "classnames";

import Editor from "../containers/Editor";

import "../assets/css/EditorPanel.css";

class EditorPanel extends React.Component {
  constructor(props) {
    super(props);

    this.state = {
      action: "query"
    };

    this.handleChange = this.handleChange.bind(this);
  }

  handleChange(event) {
    this.setState({
      action: event.target.dataset.action
    });
  }
  render() {
    const {
      canDiscardAll,
      query,
      onRunQuery,
      onUpdateQuery,
      onClearQuery,
      onDiscardAllFrames,
      saveCodeMirrorInstance,
      connected
    } = this.props;

    const isQueryDirty = query.trim() !== "";

    return (
      <div className="editor-panel">
        <div className="header">
          <div
            className={classnames("status", {
              connected,
              "not-connected": !connected
            })}
          >
            <i className="fa fa-circle status-icon" />
            <span className="status-text">
              {connected ? "Connected" : "Not connected"}
            </span>
          </div>
          <div className="actions">
            <a
              href="#"
              className={classnames("action clear-btn", {
                actionable: canDiscardAll
              })}
              onClick={e => {
                e.preventDefault();

                if (confirm("Are you sure? This will close all frames.")) {
                  onDiscardAllFrames();
                }
              }}
            >
              <i className="fa fa-trash" /> Close all
            </a>
            <a
              href="#"
              className={classnames("action clear-btn", {
                actionable: isQueryDirty
              })}
              onClick={e => {
                e.preventDefault();
                if (query === "") {
                  return;
                }

                onClearQuery();
              }}
            >
              <i className="fa fa-close" /> Clear
            </a>
            <a
              href="#"
              className={classnames("action run-btn", {
                actionable: isQueryDirty
              })}
              onClick={e => {
                e.preventDefault();
                if (query === "") {
                  return;
                }

                onRunQuery(query, this.state.action);
              }}
            >
              <i className="fa fa-play" /> Run
            </a>
          </div>
        </div>

        <Editor
          onUpdateQuery={onUpdateQuery}
          onRunQuery={onRunQuery}
          query={query}
          action={this.state.action}
          saveCodeMirrorInstance={saveCodeMirrorInstance}
        />
        <div className="editor-radio">
          <form>
            <label>
              <input
                className="editor-type"
                type="radio"
                name="action"
                data-action="query"
                onChange={this.handleChange}
              />Query
            </label>
            <label>
              <input
                className="editor-type"
                type="radio"
                name="action"
                data-action="mutate"
                onChange={this.handleChange}
              />Mutate
            </label>
            <label>
              <input
                className="editor-type"
                type="radio"
                name="action"
                data-action="alter"
                onChange={this.handleChange}
              />Alter
            </label>
          </form>
        </div>
      </div>
    );
  }
}

function mapStateToProps(state) {
  return {
    query: state.query.query
  };
}

export default connect(mapStateToProps)(EditorPanel);
