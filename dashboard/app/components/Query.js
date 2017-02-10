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

function replay() {
  console.log("replaying")
}

var Query = React.createClass({
  render: function() {
    return (
      <div className="query" style={{marginBottom: '20px', paddingBottom: '10px', borderBottom: '1px solid gray'}}>
      <div style={{ padding: '5px'}}>
        <pre onClick={this.props.update}>{prettifyQuery(this.props.text)}</pre>
      </div>
      </div>
    )
  }
});

module.exports = Query;
