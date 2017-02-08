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
      <div onClick={this.props.update} className="query" style={{marginBottom: '20px', borderBottom: '1px solid lightgray'}}>
      <pre>{prettifyQuery(this.props.text)}</pre> 
      </div>
    )
  }
});

module.exports = Query;
