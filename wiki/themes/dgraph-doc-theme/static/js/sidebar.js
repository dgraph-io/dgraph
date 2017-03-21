(function () {
  var h2s = document.querySelectorAll('h2');
  var h3s = document.querySelectorAll('h3');

  // TODO: h2sWithH3s
  var h2sWithH3s = [];

  for (var i = 0; i < h2s.length; i++) {
    var h2 = h2s[i];

    h2sWithH3s.push(h2);
  }

  console.log('h2sWithH3s', h2sWithH3s);

  if (h2sWithH3s.length > 0) {
    subMenu = document.creatElement('ul');

  }
})();
