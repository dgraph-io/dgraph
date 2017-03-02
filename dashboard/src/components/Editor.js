import React, { Component } from "react";

import { timeout, checkStatus, parseJSON } from "./Helpers";
import "../assets/css/Editor.css";

require("codemirror/addon/hint/show-hint.css");

function isNotEmpty(response) {
  let keys = Object.keys(response);
  for (let i = 0; i < keys.length; i++) {
    if (keys[i] !== "server_latency" && keys[i] !== "uids") {
      return keys[i].length > 0;
    }
  }
  return false;
}

function handleResponse(result) {
  // This is the case in which user sends a mutation. We display the response from server.
  if (result.code !== undefined && result.message !== undefined) {
    this.props.storeLastQuery();
    this.props.renderResText("success-res", JSON.stringify(result, null, 2));
  } else if (isNotEmpty(result)) {
    this.props.storeLastQuery();
    let query = this.getValue(),
      mantainSortOrder = query.indexOf("orderasc") !== -1 ||
        query.indexOf("orderdesc") !== -1;
    this.props.renderGraph(result, mantainSortOrder);
  } else {
    // We probably didn't get any results.
    this.props.renderResText(
      "error-res",
      "Your query did not return any results.",
    );
  }
}

class Editor extends Component {
  getValue = () => {
    return this.editor.getValue();
  };

  runQuery = (e: Event) => {
    e.preventDefault();
    this.props.updateQuery(this.getValue());
    // Resetting state of parent related to a query.
    this.props.resetState();

    var that = this;
    timeout(
      60000,
      fetch(process.env.REACT_APP_DGRAPH + "/query?debug=true", {
        method: "POST",
        mode: "cors",
        headers: {
          "Content-Type": "text/plain",
        },
        body: that.getValue(),
      })
        .then(checkStatus)
        .then(parseJSON)
        .then(handleResponse.bind(that)),
    )
      .catch(function(error) {
        console.log(error.stack);
        var err = error.response && (error.response.text() || error.message);
        return err;
      })
      .then(function(errorMsg) {
        if (errorMsg !== undefined) {
          that.props.renderResText("error-res", errorMsg);
        }
      });
  };

  render() {
    return (
      <div>
        <form id="query-form">
          <div className="form-group">
            <label htmlFor="query">Query</label>
            <button
              type="submit"
              className="btn btn-primary pull-right"
              onClick={this.runQuery}
            >
              Run
            </button>
          </div>
        </form>

        <div
          className="Editor-basic"
          ref={editor => {
            this._editor = editor;
          }}
        />
      </div>
    );
  }

  componentWillReceiveProps = nextProps => {
    if (nextProps.query !== this.getValue()) {
      this.editor.setValue(nextProps.query);
    }
  };

  componentDidMount = () => {
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
      fetch(process.env.REACT_APP_DGRAPH + "/ui/keywords", {
        method: "GET",
        mode: "cors",
      })
        .then(checkStatus)
        .then(parseJSON)
        .then(function(result) {
          keywords = result.keywords.map(function(kw) {
            return kw.name;
          });
        }),
    )
      .catch(function(error) {
        console.log(error.stack);
        console.warn(
          "In catch: Error while trying to fetch list of keywords",
          error,
        );
        return error;
      })
      .then(function(errorMsg) {
        if (errorMsg !== undefined) {
          console.warn(
            "Error while trying to fetch list of keywords",
            errorMsg,
          );
        }
      });

    this.editor = CodeMirror(this._editor, {
      value: this.props.query,
      lineNumbers: true,
      tabSize: 2,
      lineWrapping: true,
      mode: "graphql",
      theme: "graphiql",
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
          this.runQuery(new Event(""));
        },
        "Ctrl-Enter": () => {
          this.runQuery(new Event(""));
        },
      },
      autofocus: true,
    });

    this.editor.setCursor(this.editor.lineCount(), 0);

    CodeMirror.registerHelper("hint", "fromList", function(cm, options) {
      var cur = cm.getCursor(), token = cm.getTokenAt(cur);

      // This is so that we automatically have a space before (, so that auto-
      // complete inside braces works. Otherwise it doesn't work for
      // director.film(orderasc: release_date).
      let openBrac = token.string.indexOf("(");
      if (
        openBrac !== -1 &&
        token.string[openBrac - 1] !== undefined &&
        token.string[openBrac - 1] !== " "
      ) {
        cm.replaceRange(" ", { line: cur.line, ch: token.start + openBrac });
      }

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

      if (found.length) return { list: found, from: from, to: to };
    });

    CodeMirror.commands.autocomplete = function(cm) {
      CodeMirror.showHint(cm, CodeMirror.hint.fromList, {
        completeSingle: false,
        words: keywords,
      });
    };

    this.editor.on("keydown", function(cm, event) {
      const code = event.keyCode;

      if (!event.ctrlKey && code >= 65 && code <= 90) {
        CodeMirror.commands.autocomplete(cm);
      }
    });
  };
}

export default Editor;
