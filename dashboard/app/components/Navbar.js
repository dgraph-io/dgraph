var React = require('react');
var Navbar = require('react-bootstrap').Navbar;
var Nav = require('react-bootstrap').Nav;
var NavItem = require('react-bootstrap').NavItem;

var NavBar = React.createClass({
  render: function() {
    return (
      <Navbar style={{borderBottom: '0.5px solid gray'}}>
		<Navbar.Header>
	      <Navbar.Brand>
	        <a href="#" style={{paddingTop: '10px'}}><img src="assets/images/logo.svg" width="100" height="30" alt=""/></a>
	      </Navbar.Brand>
	    </Navbar.Header>
	    <Nav>
	      <NavItem eventKey={1} href="#">Visualization</NavItem>
	    </Nav>
	  </Navbar>
    )
  }
});

module.exports = NavBar;
