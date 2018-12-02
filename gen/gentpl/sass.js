var sass = require('node-sass');
var manifest = require('./manifest.json');

module.exports = {
  'asset($url)' : function(url) {
    var v = url.getValue();
    if (manifest[v] !== undefined) {
      return sass.types.String("url('/_/" + manifest[v] + "')");
    }
    return url;
  }
}
