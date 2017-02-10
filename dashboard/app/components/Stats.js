var React = require('react');
var stats = require('../styles').statistics;

var Stats = React.createClass({
  render: function() {
    return (
      <div style={stats} className={this.props.class}>
      		<span>{this.props.latency != '' && this.props.rendering != '' ? '* Server Latency - ' + this.props.latency + ', Rendering - ' + this.props.rendering : ''}</span>
      	</div>
    )
  }
});

module.exports = Stats;
