var React = require('react');
var stats = require('../styles').statistics;

var Stats = React.createClass({
  render: function() {
    return (
      <div style={stats} className={this.props.class}>
        <form>
          <div className="form-group col-sm-6">
              <label htmlFor="server_latency">Server Latency</label>
              <input type="input" className="form-control" value={this.props.latency} readOnly/>
          </div>
          <div className="form-group col-sm-6">
              <label htmlFor="rendering">Rendering</label>
              <input type="input" className="form-control" value={this.props.rendering} readOnly/>
          </div>
        </form>
      </div>
    )
  }
});

module.exports = Stats;
