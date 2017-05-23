import React, { Component } from "react";
import { connect } from "react-redux";

import { timeout, checkStatus, sortStrings, getEndpointBaseURL } from "../lib/helpers";
import "../assets/css/Editor.css";

require("codemirror/addon/hint/show-hint.css");

class Editor extends Component {
  getValue = () => {
    return this.editor.getValue();
  };

  render() {
    return (
      <div
        className="Editor-basic"
        ref={editor => {
          this._editor = editor;
        }}
      />
    );
  }

  componentWillReceiveProps = nextProps => {
    if (nextProps.query !== this.getValue()) {
      this.editor.setValue(nextProps.query);
    }
  };

  componentDidMount = () => {
    const { saveCodeMirrorInstance } = this.props;

    const CodeMirror = require("codemirror");
    require("codemirror/addon/hint/show-hint");
    require("codemirror/addon/comment/comment");
    require("codemirror/addon/edit/matchbrackets");
    require("codemirror/addon/edit/closebrackets");
    require("codemirror/addon/fold/foldcode");
    require("codemirror/addon/fold/foldgutter");
    require("codemirror/addon/fold/brace-fold");
    require("codemirror/addon/lint/lint");
    require("codemirror/keymap/sublime");
    require("codemirror-graphql/hint");
    require("codemirror-graphql/lint");
    require("codemirror-graphql/info");
    require("codemirror-graphql/jump");
    require("codemirror-graphql/mode");

    let keywords = [];
    timeout(
      1000,
      fetch(getEndpointBaseURL() + "/ui/keywords", {
        method: "GET",
        mode: "cors"
      })
        .then(checkStatus)
        .then(response => response.json())
        .then(function(result) {
          keywords = keywords.concat(
            result.keywords.map(kw => {
              return kw.name;
            })
          );
        })
    )
      .catch(function(error) {
        console.log(error.stack);
        console.warn(
          "In catch: Error while trying to fetch list of keywords",
          error
        );
        return error;
      })
      .then(function(errorMsg) {
        if (errorMsg !== undefined) {
          console.warn(
            "Error while trying to fetch list of keywords",
            errorMsg
          );
        }
      });

    timeout(
      1000,
      fetch(getEndpointBaseURL() + "/query", {
        method: "POST",
        mode: "cors",
        body: "schema {}"
      })
        .then(checkStatus)
        .then(response => response.json())
        .then(function(result) {
          if (result.schema && result.schema.length !== 0) {
            keywords = keywords.concat(
              result.schema.map(kw => {
                return kw.predicate;
              })
            );
          }
        })
    )
      .catch(function(error) {
        console.log(error.stack);
        console.warn("In catch: Error while trying to fetch schema", error);
        return error;
      })
      .then(function(errorMsg) {
        if (errorMsg !== undefined) {
          console.warn("Error while trying to fetch schema", errorMsg);
        }
      });

    this.editor = CodeMirror(this._editor, {
      value: this.props.query,
      lineNumbers: true,
      tabSize: 2,
      lineWrapping: true,
      mode: "graphql",
      theme: "neo",
      keyMap: "sublime",
      autoCloseBrackets: true,
      completeSingle: false,
      showCursorWhenSelecting: true,
      foldGutter: true,
      gutters: ["CodeMirror-linenumbers", "CodeMirror-foldgutter"],
      extraKeys: {
        "Ctrl-Space": cm => {
          CodeMirror.commands.autocomplete(cm);
        },
        "Cmd-Space": cm => {
          CodeMirror.commands.autocomplete(cm);
        },
        "Cmd-Enter": () => {
          this.props.onRunQuery(this.getValue());
        },
        "Ctrl-Enter": () => {
          this.props.onRunQuery(this.getValue());
        }
      },
      autofocus: true
    });

    this.editor.setCursor(this.editor.lineCount(), 0);

    CodeMirror.registerHelper("hint", "fromList", function(cm, options) {
      var cur = cm.getCursor(), token = cm.getTokenAt(cur);

      var to = CodeMirror.Pos(cur.line, token.end);
      let from = "", term = "";
      if (token.string) {
        term = token.string;
        from = CodeMirror.Pos(cur.line, token.start);
      } else {
        term = "";
        from = to;
      }

      // So that we don't autosuggest for anyof/allof filter values which
      // would be inside quotes.
      if (term.length > 0 && term[0] === '"') {
        return { list: [], from: from, to: to };
      }

      // TODO - This is a hack because Graphiql mode considers . as an invalidchar.
      // Ideally we should write our own mode which allows . in predicate.
      if (
        token.type === "invalidchar" &&
        token.state.prevState !== undefined &&
        token.state.prevState.kind === "Field"
      ) {
        term = token.state.prevState.name + token.string;
        from.ch -= token.state.prevState.name.length;
      }

      // because Codemirror strips the @ from a directive.
      if (token.state.kind === "Directive") {
        term = "@" + term;
        from.ch -= 1;
      }

      term = term.toLowerCase();

      var found = [];
      for (var i = 0; i < options.words.length; i++) {
        var word = options.words[i];
        if (term.length > 0 && word.startsWith(term)) {
          found.push(word);
        }
      }

      if (found.length) {
        return {
          list: found.sort(sortStrings),
          from: from,
          to: to
        };
      }
    });

    CodeMirror.commands.autocomplete = function(cm) {
      CodeMirror.showHint(cm, CodeMirror.hint.fromList, {
        completeSingle: false,
        words: keywords
      });
    };

    this.editor.on("change", cm => {
      const { onUpdateQuery } = this.props;
      if (!onUpdateQuery) {
        return;
      }

      const val = this.editor.getValue();
      onUpdateQuery(val);
    });

    this.editor.on("keydown", function(cm, event) {
      const code = event.keyCode;

      if (!event.ctrlKey && code >= 65 && code <= 90) {
        CodeMirror.commands.autocomplete(cm);
      }
    });

    if (saveCodeMirrorInstance) {
      saveCodeMirrorInstance(this.editor);
    }
  };
}

const mapStateToProps = state => ({
});

const mapDispatchToProps = {
};

export default connect(mapStateToProps, mapDispatchToProps)(Editor);
