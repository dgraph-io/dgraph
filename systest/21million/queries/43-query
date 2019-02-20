{
  var(func:allofterms(name@en, "angelina jolie")) {
    name@en
    actor.film {
      A AS performance.film {
        B AS genre
      }
    }
  }

  films(func: uid(B), orderasc: name@en) {
    name@en
    ~genre @filter(uid(A)) {
      name@en
    }
  }
}
