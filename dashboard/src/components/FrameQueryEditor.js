import React from 'react';
import classnames from 'classnames';

import Editor from '../containers/Editor';

export default class FrameQueryEditor extends React.Component {
  constructor(props) {
    super(props);

    this.state = {
      currentQuery: props.query
    };
  }

  handleUpdateQuery = (val) => {
    this.setState({ currentQuery: val });
  }

  onRunQuery = () => {
    const { handleRunQuery, onToggleEditingQuery, frame } = this.props;
    const { currentQuery } = this.state;

    handleRunQuery(currentQuery, frame.id, () => {
      onToggleEditingQuery();
    });
  }

  render() {
    const {
      query, open, onToggleEditingQuery, saveCodeMirrorInstance
    } = this.props;
    const { currentQuery } = this.state;

    return (
      <div className={classnames('frame-query-editor', { open })}>
        <Editor
          query={currentQuery}
          onUpdateQuery={this.handleUpdateQuery}
          onRunQuery={this.onRunQuery}
          saveCodeMirrorInstance={saveCodeMirrorInstance}
        />
        <div className="actions">
          <a
            href="#discard"
            className="btn btn-default"
            onClick={(e) => {
              e.preventDefault();

              this.setState({ currentQuery: query }, () => {
                onToggleEditingQuery();
              });
            }}
          >
            Discard
          </a>
          <a
            href="#run"
            className="btn btn-primary"
            onClick={(e) => {
              e.preventDefault();

              this.onRunQuery();
            }}
          >
            Run
          </a>
        </div>
      </div>
    );
  }
}
