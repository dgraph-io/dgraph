var React = require('react');

function prettifyQuery(q) {
  var parsedQuery
  try {
    parsedQuery = JSON.parse(q);
  } catch (e) {
    return q;
  }
  return JSON.stringify(parsedQuery, null, 2);
}

function getQueryStructure(query) {
  var lines = query.split("\n");
  lines = lines.map(function(line) {
    return line.trim();
  })
  var structure = ""
  for (var i = 0; i < lines.length; i++) {
    // We execute as soon as we encounter the first }
    if (lines[i].indexOf("}") != -1) {
      break;
    }
    if (lines[i] == "{") {
      continue
    }
    var openCurly = lines[i].indexOf("{");
    if (openCurly == -1) {
      continue
    }
    var delim = " --> "
    if (structure.length === 0) {
      delim = ""
    }
    structure = structure + delim + lines[i].substr(0, openCurly).trim()
  }
  return structure
}

var Query = React.createClass({
  render: function() {
    return (
      <div className="query" style={{marginBottom: '20px', paddingBottom: '10px', borderBottom: '1px solid gray'}}>
      <div style={{ padding: '5px'}}>
        <pre onClick={this.props.update} data-query={this.props.text}>{getQueryStructure(prettifyQuery(this.props.text))}</pre>
      </div>
      </div>
    )
  }
});

module.exports = Query;
